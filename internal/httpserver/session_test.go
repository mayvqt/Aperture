package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestValidCSRFDeniesEmptyValues(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	if s.validCSRF(req, "") {
		t.Fatal("empty CSRF values should not validate")
	}
}

func TestAdminRevocationInvalidatesApertureSession(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{adminRevoked: true})
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Fatalf("status = %d, location = %q; want login redirect", rr.Code, rr.Header().Get("Location"))
	}
	if store.deletedSessionID != store.session.ID {
		t.Fatalf("deleted session = %q, want %q", store.deletedSessionID, store.session.ID)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("revocation cookies = %#v, want expired session cookie", cookies)
	}
}

func TestAdminVerificationFailsClosed(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), &fakeMediaServer{isAdminErr: errors.New("jellyfin unavailable")})
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), "Could not verify access") {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
}

func TestAdminInvalidMediaServerTokenClearsSession(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{
		isAdminErr: &mediaserver.HTTPError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"},
	})
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Fatalf("status = %d, location = %q; want login redirect", rr.Code, rr.Header().Get("Location"))
	}
	if store.deletedSessionID != store.session.ID {
		t.Fatalf("deleted session = %q, want %q", store.deletedSessionID, store.session.ID)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("invalid-token cookies = %#v, want expired session cookie", cookies)
	}
}

func TestLogoutRequiresValidCSRFBeforeClearingCookie(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodPost, "/logout", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if store.deletedSessionID != "" {
		t.Fatalf("deleted session = %q, want none", store.deletedSessionID)
	}
	if cookies := rr.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("cookies = %#v, want none", cookies)
	}
}

func TestLogoutWithValidCSRFDeletesSessionAndClearsCookie(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Fatalf("status = %d, location = %q; want login redirect", rr.Code, rr.Header().Get("Location"))
	}
	if store.deletedSessionID != store.session.ID {
		t.Fatalf("deleted session = %q, want %q", store.deletedSessionID, store.session.ID)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("logout cookies = %#v, want expired session cookie", cookies)
	}
}
