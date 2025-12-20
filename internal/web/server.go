package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ssuji15/wolf/internal/component"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Server struct {
	router     chi.Router
	jobService *jobservice.JobService
}

func NewServer(comp *component.Components) *Server {

	s := &Server{
		router:     chi.NewRouter(),
		jobService: jobservice.NewJobService(comp),
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
	r.Use(middleware.Timeout(2 * time.Second)) // 2s per request timeout
	r.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "WebServer")
	})

	r.Post("/job", s.handleCreateJob)
	r.Get("/job/{id}", s.handleGetJob)
	r.Get("/job", s.handleListJob)
	r.Get("/job/{id}/output", s.handleDownloadOutput)
	r.Get("/job/{id}/code", s.handleDownloadCode)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {

	r.Body = http.MaxBytesReader(w, r.Body, int64(1.5*1024*1024)) // 1.5MB
	defer r.Body.Close()

	var req model.JobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	job, err := s.jobService.CreateJob(r.Context(), req)
	if err != nil {
		http.Error(w, "failed to create job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	response, err := s.jobService.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to get job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleListJob(w http.ResponseWriter, r *http.Request) {
	response, err := s.jobService.ListJobs(r.Context())
	if err != nil {
		http.Error(w, "failed to list job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleDownloadOutput(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	response, err := s.jobService.DownloadOutput(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to get job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="output.bin"`)
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (s *Server) handleDownloadCode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	response, err := s.jobService.DownloadCode(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to get job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="output.bin"`)
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}
