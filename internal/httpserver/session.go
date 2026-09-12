package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/connection"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/security"
)

const (
	sessionCookieName   = "aperture_session"
	publicCSRFCookie    = "aperture_public_csrf"
	anonymousCSRFCookie = "aperture_form_csrf"
	csrfMaxAge          = 3600
	adminCheckTimeout   = 5 * time.Second
)

func (s *Server) admin(next func(http.ResponseWriter, *http.Request, db.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		complete, err := s.setupComplete(r.Context())
		if err != nil {
			s.error(w, err)
			return
		}
		if !complete {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		session, ok, err := s.session(r)
		if err != nil {
			s.error(w, err)
			return
		}
		if !ok {
			http.SetCookie(w, s.sessionCookie("", -time.Hour))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		snapshot, err := s.snapshot(r.Context())
		if err != nil {
			s.error(w, err)
			return
		}
		matches := func(candidate connection.Snapshot) bool {
			return session.BindingID > 0 && session.BindingID == candidate.Identity.Binding.ID && session.Generation == candidate.Identity.Generation
		}
		if !matches(snapshot) {
			s.clearSession(w, r, session.ID)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		verified, err := s.connections.Verify(r.Context(), snapshot, session.AccessToken, session.DeviceID)
		if err != nil {
			if invalidMediaSession(err) || errors.Is(err, db.ErrConnectionChanged) {
				s.clearSession(w, r, session.ID)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			s.message(w, "Could not verify server", "Aperture could not confirm the media server's identity. Try again shortly.", http.StatusBadGateway)
			return
		}
		if !matches(verified) {
			s.clearSession(w, r, session.ID)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		r = r.WithContext(connection.WithSnapshot(r.Context(), verified))
		checkCtx, cancel := context.WithTimeout(r.Context(), adminCheckTimeout)
		isAdmin, err := verified.Media.IsAdmin(checkCtx, verified.Settings.ServerURL, session.AccessToken, session.DeviceID, session.UserID)
		cancel()
		if err != nil {
			if invalidMediaSession(err) {
				s.clearSession(w, r, session.ID)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			s.message(w, "Could not verify access", "Aperture could not confirm your media-server administrator access. Try again shortly.", http.StatusBadGateway)
			return
		}
		if !isAdmin {
			s.clearSession(w, r, session.ID)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, session)
	}
}
func (s *Server) adminPost(next func(http.ResponseWriter, *http.Request, db.Session)) http.HandlerFunc {
	return s.admin(func(w http.ResponseWriter, r *http.Request, session db.Session) {
		if !s.validCSRF(r, session.CSRFSecret) {
			s.message(w, "Invalid request", "Refresh the page and try again.", http.StatusBadRequest)
			return
		}
		next(w, r, session)
	})
}
func (s *Server) session(r *http.Request) (db.Session, bool, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return db.Session{}, false, nil
	}
	session, err := s.store.Session(r.Context(), cookie.Value)
	if errors.Is(err, db.ErrNotFound) {
		return db.Session{}, false, nil
	}
	if err != nil {
		return db.Session{}, false, err
	}
	return session, true, nil
}
func (s *Server) data(r *http.Request, session db.Session) viewData {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	page, title := path, "Aperture"
	switch {
	case path == "/admin":
		page, title = "/admin", "Dashboard"
	case strings.HasPrefix(path, "/admin/invites"):
		page, title = "/admin/invites", "Invites"
	case strings.HasPrefix(path, "/admin/templates"):
		page, title = "/admin/templates", "Templates"
	case strings.HasPrefix(path, "/admin/registrations"):
		page, title = "/admin/registrations", "Registrations"
	case strings.HasPrefix(path, "/admin/users"):
		page, title = "/admin/users", "Users"
	case strings.HasPrefix(path, "/admin/webhooks"):
		page, title = "/admin/webhooks", "Webhooks"
	case strings.HasPrefix(path, "/admin/audit"):
		page, title = "/admin/audit", "Audit log"
	case strings.HasPrefix(path, "/admin/settings"):
		page, title = "/admin/settings", "Settings"
	}
	return viewData{BindingID: session.BindingID, Admin: true, Username: session.Username, CSRF: session.CSRFSecret, Title: title, CurrentPage: page}
}
func (s *Server) validCSRF(r *http.Request, expected string) bool {
	actual, ok := formCSRF(r)
	if expected == "" || actual == "" {
		return false
	}
	return ok && security.ConstantEqual(expected, actual)
}
func (s *Server) publicCSRF(w http.ResponseWriter, r *http.Request, token string) (string, error) {
	settings, err := s.settings(r.Context())
	if err != nil {
		return "", err
	}
	if settings.InviteSecret == "" {
		return "", errors.New("invite secret is not configured")
	}
	return s.signedCSRF(w, publicCSRFCookie, "/i/", settings.InviteSecret, token+":")
}
func (s *Server) anonymousCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	settings, err := s.settings(r.Context())
	if err != nil {
		return "", err
	}
	if settings.SessionSecret == "" {
		return "", errors.New("session secret is not configured")
	}
	return s.signedCSRF(w, anonymousCSRFCookie, "/", settings.SessionSecret, "")
}
func (s *Server) signedCSRF(w http.ResponseWriter, cookieName, path, secret, prefix string) (string, error) {
	value, err := csrfValue()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    security.HashToken(secret, prefix+value),
		Path:     path,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(),
		MaxAge:   csrfMaxAge,
	})
	return value, nil
}
func (s *Server) validAnonymousCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(anonymousCSRFCookie)
	if err != nil {
		return false
	}
	settings, err := s.settings(r.Context())
	if err != nil || settings.SessionSecret == "" {
		return false
	}
	return validSignedRequestCSRF(r, cookie.Value, settings.SessionSecret, "")
}
func (s *Server) validPublicCSRF(r *http.Request, token string) bool {
	cookie, err := r.Cookie(publicCSRFCookie)
	if err != nil {
		return false
	}
	settings, err := s.settings(r.Context())
	if err != nil || settings.InviteSecret == "" {
		return false
	}
	return validSignedRequestCSRF(r, cookie.Value, settings.InviteSecret, token+":")
}
func validSignedRequestCSRF(r *http.Request, cookieValue, secret, prefix string) bool {
	formValue, ok := formCSRF(r)
	return ok && validSignedCSRF(cookieValue, formValue, secret, prefix)
}
func validSignedCSRF(cookieValue, formValue, secret, prefix string) bool {
	if cookieValue == "" || formValue == "" || secret == "" {
		return false
	}
	return security.ConstantEqual(cookieValue, security.HashToken(secret, prefix+formValue))
}
func formCSRF(r *http.Request) (string, bool) {
	if err := r.ParseForm(); err != nil {
		return "", false
	}
	return r.FormValue("csrf"), true
}
func (s *Server) sessionCookie(value string, maxAge time.Duration) *http.Cookie {
	expires := time.Now().Add(maxAge)
	if maxAge < 0 {
		expires = time.Unix(1, 0)
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(),
		MaxAge:   int(maxAge.Seconds()),
		Expires:  expires,
	}
}
func (s *Server) secureCookie() bool {
	_, _, secure := s.runtimeSettings()
	return secure
}

func csrfValue() (string, error) {
	return security.RandomToken(32)
}

func invalidMediaSession(err error) bool {
	var httpErr *mediaserver.HTTPError
	return errors.As(err, &httpErr) &&
		(httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden)
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	_ = s.store.DeleteSession(r.Context(), sessionID)
	http.SetCookie(w, s.sessionCookie("", -time.Hour))
}
