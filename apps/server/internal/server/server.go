package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Address string
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

type Capabilities struct {
	Version       string   `json:"version"`
	Features      []string `json:"features"`
	RefNamespaces []string `json:"ref_namespaces"`
}

func New(cfg Config) *Server {
	if cfg.Address == "" {
		cfg.Address = ":8000"
	}

	s := &Server{
		cfg: cfg,
		mux: http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.cfg.Address, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/events/stream", s.handleEvents)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	payload := Capabilities{
		Version:  "v1",
		Features: []string{"workspaces", "changes", "attestations", "suggestions"},
		RefNamespaces: []string{
			"refs/jul/workspaces",
			"refs/jul/keep",
			"refs/jul/suggest",
			"refs/notes/jul",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to encode"))
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("streaming unsupported"))
		return
	}

	ctx := r.Context()
	_, _ = fmt.Fprintf(w, "event: ready\ndata: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			_, _ = fmt.Fprintf(w, "event: ping\ndata: %s\n\n", t.UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}
