// Package server implements the HTTP server for TQL queries (D8).
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"oneshottest/internal/tql"
)

// Server provides HTTP endpoints for TQL queries.
type Server struct {
	router   chi.Router
	executor *tql.Executor
}

// New creates a new Server with TQL executor (D8, D9).
func New(executor *tql.Executor) *Server {
	r := chi.NewRouter()
	s := &Server{
		router:   r,
		executor: executor,
	}

	// Mount routes.
	r.Post("/query", s.handleQuery)
	r.Get("/healthz", s.handleHealthz)

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// handleQuery processes POST /query requests (D8).
// Request body: raw TQL text
// Response: JSON {"results":[...], "error"?: "..."}
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Read TQL text from body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(tql.Result{Error: "failed to read request body"})
		return
	}
	defer r.Body.Close()

	tqlText := strings.TrimSpace(string(body))
	if tqlText == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(tql.Result{Error: "empty query"})
		return
	}

	// Parse TQL.
	stmt, err := tql.Parse(tqlText)
	if err != nil {
		// Parse error is a client mistake (400).
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(tql.Result{Error: err.Error()})
		return
	}

	// Execute.
	result, err := s.executor.Exec(r.Context(), stmt)
	if err != nil {
		// Execution error is a server error (500).
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(tql.Result{Error: err.Error()})
		return
	}

	// Success.
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// handleHealthz responds to liveness checks (D8).
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
