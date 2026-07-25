package httpserver

import "net/http"

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ok, err := s.setupComplete(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if !ok {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
