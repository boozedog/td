package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/marcus/td/internal/serverdb"
)

// Server is the HTTP API server for td-sync.
type Server struct {
	config Config
	http   *http.Server
	store  *serverdb.ServerDB
	dbPool *ProjectDBPool
}

// NewServer creates a new Server with the given config and store.
func NewServer(cfg Config, store *serverdb.ServerDB) (*Server, error) {
	s := &Server{
		config: cfg,
		store:  store,
		dbPool: NewProjectDBPool(cfg.ProjectDataDir),
	}

	s.http = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      s.routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// Start begins listening for HTTP requests (non-blocking).
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the server and closes all project databases.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	s.dbPool.CloseAll()
	return err
}

// routes builds the HTTP handler with all routes and middleware.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Projects
	mux.HandleFunc("POST /v1/projects", s.requireAuth(s.handleCreateProject))
	mux.HandleFunc("GET /v1/projects", s.requireAuth(s.handleListProjects))
	mux.HandleFunc("GET /v1/projects/{id}", s.requireProjectAuth(serverdb.RoleReader, s.handleGetProject))
	mux.HandleFunc("PATCH /v1/projects/{id}", s.requireProjectAuth(serverdb.RoleWriter, s.handleUpdateProject))
	mux.HandleFunc("DELETE /v1/projects/{id}", s.requireProjectAuth(serverdb.RoleOwner, s.handleDeleteProject))

	// Members
	mux.HandleFunc("POST /v1/projects/{id}/members", s.requireProjectAuth(serverdb.RoleOwner, s.handleAddMember))
	mux.HandleFunc("GET /v1/projects/{id}/members", s.requireProjectAuth(serverdb.RoleReader, s.handleListMembers))
	mux.HandleFunc("PATCH /v1/projects/{id}/members/{userID}", s.requireProjectAuth(serverdb.RoleOwner, s.handleUpdateMember))
	mux.HandleFunc("DELETE /v1/projects/{id}/members/{userID}", s.requireProjectAuth(serverdb.RoleOwner, s.handleRemoveMember))

	// Sync
	mux.HandleFunc("POST /v1/projects/{id}/sync/push", s.requireProjectAuth(serverdb.RoleWriter, s.handleSyncPush))
	mux.HandleFunc("GET /v1/projects/{id}/sync/pull", s.requireProjectAuth(serverdb.RoleReader, s.handleSyncPull))
	mux.HandleFunc("GET /v1/projects/{id}/sync/status", s.requireProjectAuth(serverdb.RoleReader, s.handleSyncStatus))

	return chain(mux, recoveryMiddleware, requestIDMiddleware, loggingMiddleware, maxBytesMiddleware(10<<20))
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
