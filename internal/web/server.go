package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/model"
)

type Server struct {
	router     chi.Router
	jobService *service.JobService
}

func NewServer(dbClient *db.DB, storageClient storage.Storage, qClient queue.Queue, cache cache.Cache) *Server {
	s := &Server{
		router:     chi.NewRouter(),
		jobService: service.NewJobService(dbClient, storageClient, qClient, cache),
	}

	s.routes()
	return s
}

// Expose the router for main.go
func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) routes() {
	r := s.router

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60)) // 60s per request timeout

	r.Post("/job", s.handleCreateJob)
	r.Get("/job/{id}", s.handleGetJob)
	r.Get("/job", s.handleListJob)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req model.JobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	job, err := s.jobService.CreateJob(ctx, req)
	if err != nil {
		http.Error(w, "failed to create job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id := chi.URLParam(r, "id")

	response, err := s.jobService.GetJob(ctx, id)
	if err != nil {
		http.Error(w, "failed to get job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleListJob(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := s.jobService.ListJobs(ctx)
	if err != nil {
		http.Error(w, "failed to get job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
