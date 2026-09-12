package httpserver

import (
	"context"
	"github.com/mayvqt/aperture/internal/connection"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/config"
)

var htmlAttributePattern = regexp.MustCompile(`(?i)([a-z_:][a-z0-9_.:-]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>]+)))?`)

func hiddenCSRF(body string) string {
	for _, attributes := range htmlElementAttributes(body, "input") {
		if attributes["name"] == "csrf" {
			return attributes["value"]
		}
	}
	return ""
}

func htmlElementHasAttributes(body, tag string, expected map[string]string) bool {
	for _, attributes := range htmlElementAttributes(body, tag) {
		if attributesMatch(attributes, expected) {
			return true
		}
	}
	return false
}

func htmlFormHasElement(body string, formAttributes map[string]string, tag string, elementAttributes map[string]string) bool {
	formPattern := regexp.MustCompile(`(?is)<form(?:\s[^>]*)?>.*?</form>`)
	for _, form := range formPattern.FindAllString(body, -1) {
		if htmlElementHasAttributes(form, "form", formAttributes) &&
			htmlElementHasAttributes(form, tag, elementAttributes) {
			return true
		}
	}
	return false
}

func htmlElementAttributes(body, tag string) []map[string]string {
	tagPattern := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `(?:\s[^>]*)?>`)
	elements := tagPattern.FindAllString(body, -1)
	attributes := make([]map[string]string, 0, len(elements))
	for _, element := range elements {
		start := len(tag) + 1
		end := len(element) - 1
		if end > start && element[end-1] == '/' {
			end--
		}
		attributes = append(attributes, parseHTMLAttributes(element[start:end]))
	}
	return attributes
}

func parseHTMLAttributes(input string) map[string]string {
	attributes := make(map[string]string)
	for _, match := range htmlAttributePattern.FindAllStringSubmatch(input, -1) {
		value := ""
		for _, candidate := range match[2:] {
			if candidate != "" {
				value = candidate
				break
			}
		}
		attributes[strings.ToLower(match[1])] = value
	}
	return attributes
}

func attributesMatch(attributes, expected map[string]string) bool {
	for name, value := range expected {
		actual, ok := attributes[strings.ToLower(name)]
		if !ok || actual != value {
			return false
		}
	}
	return true
}

func adminRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-id"})
	req.RemoteAddr = "192.0.2.1:1234"
	return req
}

func testConfig() config.Config {
	return config.Config{
		PublicURL:        "https://aperture.example",
		CookieSecure:     false,
		SessionSecret:    "session-secret-with-at-least-32-characters",
		InviteSecret:     "invite-secret-with-at-least-32-characters",
		MediaProvider:    "jellyfin",
		ServerURL:        "http://media:8096",
		APIKey:           "api-key",
		PublicURLManaged: true,
		ProviderManaged:  true,
		ServerURLManaged: true,
		APIKeyManaged:    true,
	}
}

func testMediaFactory(media mediaserver.Server) connection.Factory {
	return func(mediaserver.Provider) (mediaserver.Server, error) { return media, nil }
}
func verifiedTestContext(t *testing.T, s *Server) context.Context {
	t.Helper()
	ctx, err := s.verifiedAPIContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func (s *Server) connectionsStateProvider() string {
	v, _ := s.connections.Peek()
	return v.Settings.Provider
}
