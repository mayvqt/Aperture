package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminNavShowsDashboardWithoutBrandIcon(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<a href="/admin">Dashboard</a>`) {
		t.Fatalf("body missing dashboard nav:\n%s", body)
	}
	if strings.Contains(body, "brand-mark") {
		t.Fatalf("body still contains global A icon markup:\n%s", body)
	}
}

func TestAuthPagesUseFullViewportShell(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<body class="auth-shell">`) || !strings.Contains(body, `<main class="auth-page">`) {
		t.Fatalf("auth page missing full viewport shell:\n%s", body)
	}
}

func TestStaticAssetsAndCSPUseExternalCSSAndJS(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/base.css", nil)
	assetRR := httptest.NewRecorder()
	handler.ServeHTTP(assetRR, assetReq)
	if assetRR.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200; body %s", assetRR.Code, assetRR.Body.String())
	}
	if got := assetRR.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("asset Cache-Control = %q, want no-cache", got)
	}

	pageReq := adminRequest(t, http.MethodGet, "/admin", nil)
	pageReq.Host = "aperture.example"
	pageRR := httptest.NewRecorder()
	handler.ServeHTTP(pageRR, pageReq)
	csp := pageRR.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP still allows inline assets: %q", csp)
	}
	if got := pageRR.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("Strict-Transport-Security = %q", got)
	}
	if got := pageRR.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	body := pageRR.Body.String()
	if !strings.Contains(body, `href="/assets/base.css"`) || !strings.Contains(body, `href="/assets/responsive.css"`) || !strings.Contains(body, `src="/assets/app.js"`) {
		t.Fatalf("page missing external asset references:\n%s", body)
	}
}

func TestHSTSOnlyAppliesToConfiguredHTTPSHost(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Host = "internal-proxy:8099"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q for unconfigured host", got)
	}
}
