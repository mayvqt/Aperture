package httpserver

import (
	"context"
	"github.com/mayvqt/aperture/internal/db"
	"net/http"
	"strconv"
	"time"
)

// Review is deliberately per record: sharing an invite never proves that its
// historical accounts belong to the server currently configured.
type serverReview struct {
	Kind                                           string
	ID, BindingID                                  int64
	Name, UserID, Source, Destination, AccountName string
	Expiry                                         string
}

func (s *Server) reviewRecord(ctx context.Context, kind string, id int64) (serverReview, error) {
	v := serverReview{Kind: kind, ID: id}
	switch kind {
	case "invite":
		r, err := s.store.Invite(ctx, id)
		if err != nil {
			return v, err
		}
		v.Name = r.Label
		v.BindingID = r.BindingID
	case "registration":
		r, err := s.store.Registration(ctx, id)
		if err != nil {
			return v, err
		}
		if db.IsRegistrationActive(r.Status) {
			return v, db.ErrRegistrationTransition
		}
		v.Name = r.Username
		v.UserID = r.ExternalUserID.String
		v.BindingID = r.BindingID
		v.Expiry = "No expiry"
		if r.UserDisableAt.Valid {
			v.Expiry = r.UserDisableAt.Time.UTC().Format("2006-01-02 15:04 UTC")
		}
	case "user":
		r, err := s.store.ManagedUser(ctx, id)
		if err != nil {
			return v, err
		}
		v.Name = r.Username
		v.UserID = r.ExternalUserID
		v.BindingID = r.BindingID
	default:
		return v, db.ErrNotFound
	}
	return v, nil
}

func (s *Server) serverReview(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.registrationRecoveryError(w, db.ErrNotFound)
		return
	}
	kind := r.PathValue("kind")
	release, err := s.claimAccountOperations(id)
	if err != nil {
		s.registrationRecoveryError(w, err)
		return
	}
	defer release()
	v, err := s.reviewRecord(r.Context(), kind, id)
	if err != nil {
		s.registrationRecoveryError(w, err)
		return
	}
	op, err := operationSnapshot(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if v.BindingID == op.Identity.Binding.ID {
		s.message(w, "Already assigned", "This record already belongs to the current server.", http.StatusConflict)
		return
	}
	bindings, err := s.store.MediaBindings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	v.Source = "Unverified (record saved before server tracking)"
	for _, b := range bindings {
		if b.ID == v.BindingID {
			v.Source = b.Provider + " · " + b.BaseURL + " · " + b.ServerID
		}
	}
	b := op.Identity.Binding
	v.Destination = b.Provider + " · " + b.BaseURL + " · " + b.ServerID
	if v.UserID != "" {
		releaseUser, err := s.claimMediaUser(op.Identity.Binding.ID, v.UserID)
		if err != nil {
			s.registrationRecoveryError(w, err)
			return
		}
		defer releaseUser()
		if op.Settings.APIKey == "" {
			s.message(w, "API key required", "Save an API key to verify the account before assigning it.", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		user, found, err := op.Media.GetUser(ctx, op.Settings.ServerURL, op.Settings.APIKey, v.UserID)
		if err != nil {
			s.message(w, "Could not verify account", "Try again when the media server is available.", http.StatusBadGateway)
			return
		}
		if !found || user.ID != v.UserID || userIsAdministrator(user) {
			s.message(w, "Account cannot be assigned", "The account ID must exist on this server and have a verified non-administrator policy.", http.StatusConflict)
			return
		}
		v.AccountName = user.Name
	}
	if r.Method == http.MethodPost {
		if r.FormValue("confirmed") != "yes" || r.FormValue("binding_id") != strconv.FormatInt(b.ID, 10) || r.FormValue("source_id") != strconv.FormatInt(v.BindingID, 10) {
			s.message(w, "Review needed", "Review the current destination and confirm this assignment.", http.StatusConflict)
			return
		}
		switch kind {
		case "invite":
			err = s.store.AdoptInvite(r.Context(), id, b.ID)
		case "registration":
			err = s.store.AdoptRegistration(r.Context(), id, b.ID)
		case "user":
			err = s.store.AdoptManagedUser(r.Context(), id, b.ID)
		}
		if err != nil {
			s.registrationRecoveryError(w, err)
			return
		}
		s.audit(r, session, "server.assign", kind, strconv.FormatInt(id, 10), map[string]any{"previous_binding": v.BindingID, "binding": b.ID})
		destination := "/admin/registrations?review=1"
		if kind == "invite" {
			destination = "/admin/invites"
		}
		if kind == "user" {
			destination = "/admin/users/review"
		}
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}
	data := s.data(r, session)
	data.Review = v
	render(w, "server-review", data)
}

func (s *Server) managedUserReview(w http.ResponseWriter, r *http.Request, session db.Session) {
	before := historyCursor(r)
	users, err := s.store.ManagedUserReviewPage(r.Context(), session.BindingID, before, 51)
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.HistoryBefore = before
	if len(users) > 50 {
		users = users[:50]
		data.NextBefore = users[49].ID
	}
	data.ManagedHistory = users
	render(w, "users-review", data)
}
func historyCursor(r *http.Request) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if v < 0 {
		return 0
	}
	return v
}
