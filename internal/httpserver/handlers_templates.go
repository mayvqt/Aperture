package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func (s *Server) templatesList(w http.ResponseWriter, r *http.Request, session db.Session) {
	templates, err := s.store.ListTemplates(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.Templates = templates
	render(w, "templates", data)
}
func (s *Server) templatesCreate(w http.ResponseWriter, r *http.Request, _ db.Session) {
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The template form could not be read.", http.StatusBadRequest)
		return
	}
	template, err := templateFromForm(r, 0)
	if err != nil {
		s.message(w, "Invalid template", err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.CreateTemplate(r.Context(), template); err != nil {
		s.error(w, err)
		return
	}
	http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
}
func (s *Server) templatesShow(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid template", "That template does not exist.", http.StatusBadRequest)
		return
	}
	template, err := s.store.Template(r.Context(), id)
	if err != nil {
		s.templateError(w, err)
		return
	}
	data := s.data(r, session)
	data.Template = template
	render(w, "template-detail", data)
}
func (s *Server) templatesUpdate(w http.ResponseWriter, r *http.Request, _ db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid template", "That template does not exist.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The template form could not be read.", http.StatusBadRequest)
		return
	}
	template, err := templateFromForm(r, id)
	if err != nil {
		s.message(w, "Invalid template", err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateTemplate(r.Context(), template); err != nil {
		s.templateError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) templatesDefault(w http.ResponseWriter, r *http.Request, _ db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid template", "That template does not exist.", http.StatusBadRequest)
		return
	}
	if err := s.store.SetDefaultTemplate(r.Context(), id); err != nil {
		s.templateError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
}

func (s *Server) templatesDelete(w http.ResponseWriter, r *http.Request, _ db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid template", "That template does not exist.", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteTemplate(r.Context(), id); err != nil {
		s.templateError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
}

func (s *Server) templatesImport(w http.ResponseWriter, r *http.Request, session db.Session) {
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The import form could not be read.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	userRef := strings.TrimSpace(r.FormValue("external_user_id"))
	if name == "" || userRef == "" {
		s.message(w, "Missing import details", "Provide a template name and media-server username or user ID.", http.StatusBadRequest)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	token := settings.APIKey
	deviceID := "aperture"
	if token == "" {
		token = session.AccessToken
		deviceID = session.DeviceID
	}
	if token == "" {
		s.message(w, "Import needs media-server access", "Save an API key in settings or log in again with a media-server administrator account.", http.StatusBadRequest)
		return
	}
	imported, err := s.media.ImportTemplate(r.Context(), settings.ServerURL, token, deviceID, userRef)
	if err != nil {
		slog.Warn("template import failed", "error", safeError(err))
		status := http.StatusBadGateway
		if errors.Is(err, mediaserver.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		s.message(w, "Import failed", safeAdminImportError(err), status)
		return
	}
	template, err := cleanTemplate(db.Template{
		Name:        name,
		Description: strings.TrimSpace(r.FormValue("description")),
		PolicyJSON:  imported.PolicyJSON,
	})
	if err != nil {
		s.message(w, "Import failed", "The imported media-server template JSON could not be parsed.", http.StatusBadGateway)
		return
	}
	if err := s.store.CreateTemplate(r.Context(), template); err != nil {
		s.error(w, err)
		return
	}
	http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
}

func (s *Server) templateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		s.message(w, "Template not found", "That template does not exist.", http.StatusNotFound)
	case errors.Is(err, db.ErrTemplateIsDefault):
		s.message(w, "Template is default", "Set another template as default before deleting this one.", http.StatusConflict)
	case errors.Is(err, db.ErrTemplateInUse):
		s.message(w, "Template is in use", "Templates attached to existing invites cannot be deleted.", http.StatusConflict)
	default:
		s.error(w, err)
	}
}
