package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

var webhookEventOptions = []webhookEventOption{
	{"registration.complete", "Registration completed"},
	{"registration.failed", "Registration failed"},
	{"template.failed", "Template application failed"},
	{"template.recovered", "Template access recovered"},
	{"user.disabled", "Expired user disabled"},
	{"user.disable_failed", "Expired-user disable failed"},
}

type webhookNotice struct {
	Event, Title, Description string
	Color                     int
	Fields                    map[string]string
}
type discordPayload struct {
	Username        string                 `json:"username"`
	Content         string                 `json:"content,omitempty"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
	Embeds          []discordEmbed         `json:"embeds"`
}
type discordAllowedMentions struct {
	Roles []string `json:"roles,omitempty"`
	Parse []string `json:"parse"`
}
type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp"`
}
type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func (s *Server) notify(n webhookNotice) {
	hooks, err := s.store.ListWebhooks(context.Background())
	if err != nil {
		slog.Warn("could not list notification webhooks", "error", safeError(err))
		return
	}
	for _, hook := range hooks {
		if !hook.Enabled || !eventSelected(hook.Events, n.Event) {
			continue
		}
		hook := hook
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			if err := sendWebhook(ctx, hook, n); err != nil {
				slog.Warn("webhook delivery failed", "webhook_id", hook.ID, "event", n.Event, "error", safeError(err))
			}
		}()
	}
}

func eventSelected(csv, event string) bool {
	for _, v := range strings.Split(csv, ",") {
		if strings.TrimSpace(v) == event {
			return true
		}
	}
	return false
}
func sendWebhook(ctx context.Context, hook db.Webhook, n webhookNotice) error {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return sendWebhookWithClient(ctx, client, hook, n)
}
func sendWebhookWithClient(ctx context.Context, client *http.Client, hook db.Webhook, n webhookNotice) error {
	var payload any
	if hook.Kind == "generic" {
		payload = map[string]any{"event": n.Event, "title": n.Title, "description": n.Description, "metadata": n.Fields, "timestamp": time.Now().UTC().Format(time.RFC3339)}
	} else {
		payload = discordWebhookPayload(hook, n)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("webhook request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func discordWebhookPayload(hook db.Webhook, n webhookNotice) discordPayload {
	fields := make([]discordField, 0, len(n.Fields))
	keys := make([]string, 0, len(n.Fields))
	for k := range n.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimSpace(n.Fields[k])
		if v != "" {
			if len(v) > 1000 {
				v = v[:1000] + "…"
			}
			fields = append(fields, discordField{Name: k, Value: v, Inline: true})
		}
	}
	roles := splitRoleIDs(hook.RoleIDs)
	mentions := make([]string, len(roles))
	for i, role := range roles {
		mentions[i] = "<@&" + role + ">"
	}
	return discordPayload{Username: "Aperture", Content: strings.Join(mentions, " "), AllowedMentions: discordAllowedMentions{Roles: roles, Parse: []string{}}, Embeds: []discordEmbed{{Title: n.Title, Description: n.Description, Color: n.Color, Fields: fields, Timestamp: time.Now().UTC().Format(time.RFC3339)}}}
}

func splitRoleIDs(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
}
func normalizeRoleIDs(value string) (string, error) {
	roles := splitRoleIDs(value)
	if len(roles) > 10 {
		return "", errors.New("enter at most 10 Discord role IDs")
	}
	for _, role := range roles {
		if len(role) < 17 || len(role) > 20 {
			return "", errors.New("enter valid numeric Discord role IDs")
		}
		for _, r := range role {
			if r < '0' || r > '9' {
				return "", errors.New("enter valid numeric Discord role IDs")
			}
		}
	}
	return strings.Join(roles, ","), nil
}
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("enter a valid webhook URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return errors.New("webhook URLs must use HTTPS (HTTP is allowed only for localhost)")
	}
	if u.User != nil {
		return errors.New("webhook URLs must not contain credentials")
	}
	return nil
}

func validateWebhookKindURL(kind, raw string) error {
	if err := validateWebhookURL(raw); err != nil {
		return err
	}
	if kind != "discord" {
		return nil
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	discordHost := host == "discord.com" || strings.HasSuffix(host, ".discord.com") || host == "discordapp.com" || strings.HasSuffix(host, ".discordapp.com")
	if !discordHost || !strings.HasPrefix(u.Path, "/api/webhooks/") {
		return errors.New("enter a Discord webhook URL")
	}
	return nil
}

func (s *Server) webhooksList(w http.ResponseWriter, r *http.Request, session db.Session) {
	hooks, err := s.store.ListWebhooks(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	for i := range hooks {
		hooks[i].URL = redactWebhookURL(hooks[i].URL)
	}
	data.Webhooks = hooks
	data.WebhookEvents = webhookEventOptions
	render(w, "webhooks", data)
}
func (s *Server) webhooksCreate(w http.ResponseWriter, r *http.Request, session db.Session) {
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The webhook form could not be read.", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	endpoint := strings.TrimSpace(r.FormValue("url"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind != "discord" && kind != "generic" {
		s.message(w, "Invalid webhook", "Choose Discord or Generic JSON.", http.StatusBadRequest)
		return
	}
	roleIDs, err := normalizeRoleIDs(r.FormValue("role_ids"))
	if err != nil || (kind == "generic" && roleIDs != "") {
		if err == nil {
			err = errors.New("role mentions are available only for Discord webhooks")
		}
		s.message(w, "Invalid webhook", err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" || len([]rune(name)) > 100 {
		s.message(w, "Invalid webhook", "Enter a name of at most 100 characters.", 400)
		return
	}
	if err := validateWebhookKindURL(kind, endpoint); err != nil {
		s.message(w, "Invalid webhook", err.Error(), 400)
		return
	}
	selected := []string{}
	for _, o := range webhookEventOptions {
		if r.FormValue("event_"+o.Value) == "on" {
			selected = append(selected, o.Value)
		}
	}
	if len(selected) == 0 {
		s.message(w, "Invalid webhook", "Select at least one event.", 400)
		return
	}
	id, err := s.store.CreateWebhook(r.Context(), db.Webhook{Name: name, URL: endpoint, Kind: kind, Events: strings.Join(selected, ","), RoleIDs: roleIDs, Enabled: true})
	if err != nil {
		s.error(w, err)
		return
	}
	s.audit(r, session, "webhook.create", "webhook", strconv.FormatInt(id, 10), map[string]any{"name": name, "kind": kind, "events": selected})
	http.Redirect(w, r, "/admin/webhooks", 303)
}
func (s *Server) webhooksDelete(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid webhook", "That webhook does not exist.", 400)
		return
	}
	if err := s.store.DeleteWebhook(r.Context(), id); err != nil {
		s.error(w, err)
		return
	}
	s.audit(r, session, "webhook.delete", "webhook", strconv.FormatInt(id, 10), nil)
	http.Redirect(w, r, "/admin/webhooks", 303)
}
func (s *Server) webhooksTest(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid webhook", "That webhook does not exist.", http.StatusBadRequest)
		return
	}
	hooks, err := s.store.ListWebhooks(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	for _, hook := range hooks {
		if hook.ID != id {
			continue
		}
		err = sendWebhook(r.Context(), hook, webhookNotice{Event: "webhook.test", Title: "Webhook connected", Description: "Aperture can deliver notifications to this destination.", Color: 0x2ecc71, Fields: map[string]string{"Webhook": hook.Name, "Status": "Ready"}})
		if err != nil {
			s.message(w, "Webhook test failed", "The destination did not accept the test notification.", http.StatusBadGateway)
			return
		}
		s.audit(r, session, "webhook.test", "webhook", strconv.FormatInt(id, 10), nil)
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
		return
	}
	s.message(w, "Webhook not found", "That webhook does not exist.", http.StatusNotFound)
}
func redactWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "Saved securely"
	}
	return u.Scheme + "://" + u.Host + "/••••••"
}
