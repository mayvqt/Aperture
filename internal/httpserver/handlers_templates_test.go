package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestTemplatesCreateStripsAdminPrivilege(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	form := url.Values{
		"csrf":        {store.session.CSRFSecret},
		"name":        {"Imported"},
		"description": {"desc"},
		"policy_json": {`{"IsAdministrator":true,"EnableAllFolders":false,"EnabledFolders":["movies"]}`},
	}
	req := adminRequest(t, http.MethodPost, "/admin/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(store.createdTemplate.PolicyJSON, `"IsAdministrator":true`) {
		t.Fatalf("created policy still grants admin: %s", store.createdTemplate.PolicyJSON)
	}
	if !strings.Contains(store.createdTemplate.PolicyJSON, `"IsAdministrator":false`) {
		t.Fatalf("created policy missing forced non-admin: %s", store.createdTemplate.PolicyJSON)
	}
	if !strings.Contains(store.createdTemplate.PolicyJSON, `"EnabledFolders":["movies"]`) {
		t.Fatalf("created policy lost library access: %s", store.createdTemplate.PolicyJSON)
	}
}

func TestTemplatesPageUsesExpandableWorkflows(t *testing.T) {
	store := newFakeStore()
	store.template.IsDefault = false
	handler := New(testConfig(), store, &fakeMediaServer{})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin/templates", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<details class="panel workflow-card" open>`) ||
		!strings.Contains(body, `<details class="panel workflow-card">`) ||
		!strings.Contains(body, "Existing templates") {
		t.Fatalf("templates page is not using expandable workflow panels:\n%s", body)
	}
	if !strings.Contains(body, `data-confirm="Delete this template? This cannot be undone."`) {
		t.Fatalf("template delete form is missing confirmation:\n%s", body)
	}
}

func TestTemplateDetailShowsPreviewAndUpdatesTemplate(t *testing.T) {
	store := newFakeStore()
	store.template = db.Template{
		ID:          7,
		Name:        "Kids",
		Description: "limited",
		PolicyJSON:  `{"IsAdministrator":false,"EnabledFolders":["Movies"]}`,
	}
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodGet, "/admin/templates/7", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Preview", "Policy", "Policy summary"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `<section class="template-workflows workflow-group">`) ||
		!strings.Contains(body, `<details class="panel workflow-card" open>`) ||
		!strings.Contains(body, "Policy details") {
		t.Fatalf("template detail is missing expandable editor sections:\n%s", body)
	}
	for _, unwanted := range []string{"Configuration JSON", "Display preferences JSON", "Library access JSON"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body should not show %q:\n%s", unwanted, body)
		}
	}
	if !strings.Contains(body, `data-confirm="Delete this template? This cannot be undone."`) {
		t.Fatalf("template detail delete form is missing confirmation:\n%s", body)
	}

	form := url.Values{
		"csrf":        {store.session.CSRFSecret},
		"name":        {"Renamed"},
		"description": {"edited"},
		"policy_json": {`{"IsAdministrator":true,"EnableAllFolders":true}`},
	}
	req = adminRequest(t, http.MethodPost, "/admin/templates/7", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if location := rr.Header().Get("Location"); location != "/admin/templates/7" {
		t.Fatalf("Location = %q, want template detail", location)
	}
	if store.updatedTemplate.Name != "Renamed" || store.updatedTemplate.Description != "edited" {
		t.Fatalf("updated template = %#v", store.updatedTemplate)
	}
	if strings.Contains(store.updatedTemplate.PolicyJSON, `"IsAdministrator":true`) {
		t.Fatalf("updated policy still grants admin: %s", store.updatedTemplate.PolicyJSON)
	}
	if !strings.Contains(store.updatedTemplate.PolicyJSON, `"IsAdministrator":false`) {
		t.Fatalf("updated policy missing forced non-admin: %s", store.updatedTemplate.PolicyJSON)
	}
}

func TestTemplateDefaultAndDeleteHandlers(t *testing.T) {
	store := newFakeStore()
	store.template = db.Template{ID: 7, Name: "Kids", PolicyJSON: `{"IsAdministrator":false}`}
	handler := New(testConfig(), store, &fakeMediaServer{})

	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/templates/7/default", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("default status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if store.defaultTemplateID != 7 {
		t.Fatalf("default template id = %d, want 7", store.defaultTemplateID)
	}

	req = adminRequest(t, http.MethodPost, "/admin/templates/7/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if store.deletedTemplateID != 7 {
		t.Fatalf("deleted template id = %d, want 7", store.deletedTemplateID)
	}
}

func TestTemplateImportReturnsNotFoundForUnknownMediaUser(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{importErr: mediaserver.ErrUserNotFound})
	form := url.Values{
		"csrf":             {store.session.CSRFSecret},
		"name":             {"Imported"},
		"external_user_id": {"missing"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/templates/import-from-user", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "user was not found") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}
