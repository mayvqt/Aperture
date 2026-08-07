package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mayvqt/aperture/internal/db"
)

func (s *Server) audit(r *http.Request, session db.Session, action, targetType, targetID string, metadata any) {
	raw := []byte("{}")
	if metadata != nil {
		if encoded, err := json.Marshal(metadata); err == nil {
			raw = encoded
		}
	}
	if err := s.store.Audit(r.Context(), session.UserID, action, targetType, targetID, clientIP(r, s.trustedProxies), requestUserAgent(r), string(raw)); err != nil {
		slog.Warn("could not record audit event", "action", action, "error", safeError(err))
	}
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request, session db.Session) {
	events, err := s.store.ListAuditEvents(r.Context(), 250)
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.AuditEvents = events
	render(w, "audit", data)
}
