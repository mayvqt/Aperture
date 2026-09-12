package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminNavShowsDashboardWithoutBrandIcon(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<a href="/admin"`) || !strings.Contains(body, `>Dashboard</a>`) {
		t.Fatalf("body missing dashboard nav:\n%s", body)
	}
	if !strings.Contains(body, `<title>Dashboard · Aperture</title>`) || !strings.Contains(body, `<a href="/admin" aria-current="page">Dashboard</a>`) {
		t.Fatalf("body missing page title or current navigation state:\n%s", body)
	}
	if strings.Contains(body, "brand-mark") {
		t.Fatalf("body still contains global A icon markup:\n%s", body)
	}
	for _, want := range []string{`href="/admin/webhooks"`, `href="/admin/audit"`, `href="/admin/settings"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing direct navigation link %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "nav-more") || strings.Contains(body, ">More<") {
		t.Fatalf("body still contains More menu markup:\n%s", body)
	}
}

func TestAuthPagesUseFullViewportShell(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
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
	if !strings.Contains(body, `<section class="auth-wrap login-wrap">`) {
		t.Fatalf("login page missing compact login wrapper:\n%s", body)
	}
	if !strings.Contains(body, `<title>Sign in · Aperture</title>`) {
		t.Fatalf("login page missing descriptive document title:\n%s", body)
	}
}

func TestAuthTitlesAndAdminNavScript(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), testMediaFactory(&fakeMediaServer{}))
	req := httptest.NewRequest(http.MethodGet, "/guide", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), `<title>Account created · Aperture</title>`) {
		t.Fatalf("guide page missing descriptive document title:\n%s", rr.Body.String())
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRR := httptest.NewRecorder()
	handler.ServeHTTP(assetRR, assetReq)
	asset := assetRR.Body.String()
	for _, want := range []string{`nav.admin-nav`, `.nav-toggle`, `aria-expanded`, `Escape`, `prefers-reduced-motion`} {
		if !strings.Contains(asset, want) {
			t.Fatalf("app.js missing %q:\n%s", want, asset)
		}
	}
}

func TestStaticAssetsAndCSPUseExternalCSSAndJS(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	assetRR := httptest.NewRecorder()
	handler.ServeHTTP(assetRR, assetReq)
	if assetRR.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200; body %s", assetRR.Code, assetRR.Body.String())
	}
	if got := assetRR.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q, want long-lived immutable caching", got)
	}
	scriptReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	scriptRR := httptest.NewRecorder()
	handler.ServeHTTP(scriptRR, scriptReq)
	if got := scriptRR.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("script Cache-Control = %q, want long-lived immutable caching", got)
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
	if !strings.Contains(body, `href="/assets/app.css?v=`) || !strings.Contains(body, `src="/assets/app.js?v=`) {
		t.Fatalf("page missing external asset references:\n%s", body)
	}
}

func TestHSTSOnlyAppliesToConfiguredHTTPSHost(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), testMediaFactory(&fakeMediaServer{}))
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Host = "internal-proxy:8099"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q for unconfigured host", got)
	}
}
