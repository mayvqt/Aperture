package httpserver

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func (s *Server) usersList(w http.ResponseWriter, r *http.Request, session db.Session) {
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		s.message(w, "API key required", "Save a media-server API key before synchronizing users.", http.StatusConflict)
		return
	}
	liveUsers, err := s.media.ListUsers(r.Context(), settings.ServerURL, settings.APIKey)
	if err != nil {
		s.message(w, "Could not synchronize users", "Aperture could not read users from the media server. Try again shortly.", http.StatusBadGateway)
		return
	}
	managed, err := s.store.ListManagedUsers(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	registrations, err := s.store.RegistrationUsers(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.UserRows = mergeUserRows(liveUsers, managed, registrations)
	render(w, "users", data)
}

func (s *Server) usersTrack(w http.ResponseWriter, r *http.Request, session db.Session) {
	id := strings.TrimSpace(r.PathValue("id"))
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	users, err := s.media.ListUsers(r.Context(), settings.ServerURL, settings.APIKey)
	if err != nil {
		s.message(w, "Could not synchronize users", "Aperture could not verify that user.", http.StatusBadGateway)
		return
	}
	for _, user := range users {
		if user.ID != id {
			continue
		}
		if userIsAdministrator(user) {
			s.message(w, "Cannot manage administrator", "Aperture does not manage media-server administrator accounts.", http.StatusConflict)
			return
		}
		if err := s.store.SaveManagedUser(r.Context(), db.ManagedUser{ExternalUserID: user.ID, Username: user.Name}); err != nil {
			s.error(w, err)
			return
		}
		s.audit(r, session, "user.track", "user", user.ID, map[string]any{"username": user.Name})
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	s.message(w, "User not found", "That user no longer exists on the media server.", http.StatusNotFound)
}

func mergeUserRows(live []mediaserver.User, managed []db.ManagedUser, registrations []db.Registration) []managedUserRow {
	rows := map[string]managedUserRow{}
	for _, user := range live {
		status, class := "Active", "good"
		var policy mediaserver.Policy
		_ = json.Unmarshal(user.Policy, &policy)
		if policy.IsDisabled {
			status, class = "Disabled", "warn"
		}
		if policy.IsAdministrator {
			status, class = "Administrator", "warn"
		}
		rows[user.ID] = managedUserRow{ID: user.ID, Name: user.Name, Source: "External", Status: status, StatusClass: class, Administrator: policy.IsAdministrator}
	}
	for _, user := range managed {
		row, ok := rows[user.ExternalUserID]
		if !ok {
			row = managedUserRow{ID: user.ExternalUserID, Name: user.Username, Status: "Missing", StatusClass: "bad", Missing: true}
		}
		row.Tracked, row.Source = true, "Imported"
		rows[user.ExternalUserID] = row
	}
	for _, registration := range registrations {
		if !registration.ExternalUserID.Valid {
			continue
		}
		id := registration.ExternalUserID.String
		row, ok := rows[id]
		if !ok {
			row = managedUserRow{ID: id, Name: registration.Username, Status: "Missing", StatusClass: "bad", Missing: true}
		}
		row.Tracked, row.Source, row.RegistrationID = true, "Invite", registration.ID
		rows[id] = row
	}
	out := make([]managedUserRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func userIsAdministrator(user mediaserver.User) bool {
	var policy mediaserver.Policy
	return json.Unmarshal(user.Policy, &policy) == nil && policy.IsAdministrator
}
