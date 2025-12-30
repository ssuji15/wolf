package web

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ssuji15/wolf/internal/component"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	limiter "github.com/ssuji15/wolf/internal/web/middleware"
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
	go jobservice.PublishJobsToQueue(comp.Ctx)
	s.routes()
	return s
}

const (
	maxTotalSize = 2 * 1024 * 1024 // 1.5MB max total request
	maxCodeSize  = 1 << 20         // 1MB max code
	maxMetadata  = 64 * 1024       // 64KB max metadata JSON
)

var bufPool = sync.Pool{
	New: func() any {
		return make([]byte, maxCodeSize)
	},
}

// Expose the router for main.go
func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) routes() {

	limiter := limiter.NewLimiter(
		500, // queue size N
		40,  // max inflight K
	)

	r := s.router
	r.Use(middleware.Timeout(2 * time.Second))
	r.Use(MaxBodySizeMiddleware(maxTotalSize))
	r.Use(limiter.Limit)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxTotalSize)
	defer r.Body.Close()

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid multipart request", http.StatusBadRequest)
		return
	}

	var req model.JobRequest
	codeSize := 0

	buf := bufPool.Get().([]byte)
	defer bufPool.Put(buf)

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "failed to read multipart body", http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "metadata":
			dec := json.NewDecoder(io.LimitReader(part, maxMetadata))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				http.Error(w, "invalid metadata JSON", http.StatusBadRequest)
				return
			}
		case "code":
			for {
				if codeSize >= maxCodeSize {
					http.Error(w, "maximum code size exceeded", http.StatusBadRequest)
					return
				}
				n, err := part.Read(buf[codeSize:])
				codeSize += n

				if err == io.EOF {
					break
				}
				if err != nil {
					http.Error(w, "failed to read code", http.StatusBadRequest)
					return
				}
			}
		default:
			http.Error(w, "unexpected form field: "+part.FormName(), http.StatusBadRequest)
			return
		}
		part.Close()
	}

	if codeSize == 0 {
		http.Error(w, "empty code part", http.StatusBadRequest)
		return
	}

	req.Code = buf[:codeSize]

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

func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
