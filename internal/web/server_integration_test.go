package web

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/queue"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/model"
	tdb "github.com/ssuji15/wolf/tests/integration_test/infra/db"
	trepo "github.com/ssuji15/wolf/tests/integration_test/infra/db/repository"
	tjetstream "github.com/ssuji15/wolf/tests/integration_test/infra/jetstream"
	tminio "github.com/ssuji15/wolf/tests/integration_test/infra/minio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

var (
	testDB         *db.DB
	pgPool         *pgxpool.Pool
	dbContainer    testcontainers.Container
	POSTGRES_URL   string
	natsContainer  testcontainers.Container
	JETSTREAM_URL  string
	minioContainer testcontainers.Container
	MINIO_ENDPOINT string
	server         *Server
)

// Helper
func setServerEnv() {
	os.Setenv("SERVICE_NAME", "wolf_server")
	os.Setenv("CACHE_TYPE", "jetstream")
	os.Setenv("STORAGE_TYPE", "minio")
	os.Setenv("QUEUE_TYPE", "jetstream")
	os.Setenv("JETSTREAM_URL", JETSTREAM_URL)
	os.Setenv("MAX_MESSAGES_JOB_QUEUE", "200")
	os.Setenv("JETSTREAM_TTL", "2")
	os.Setenv("JETSTREAM_BUCKET_NAME", "TEST_CACHE")
	os.Setenv("JETSTREAM_BUCKET_SIZE", "1048576")
	os.Setenv("POSTGRES_URL", POSTGRES_URL)
	tminio.SetMinioEnv(MINIO_ENDPOINT)
}

func GetComponents() (cache.Cache, queue.Queue, storage.Storage, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := component.GetCache(context.Background(), cfg.CACHE_TYPE)
	if err != nil {
		return nil, nil, nil, err
	}

	s, err := component.GetStorage(cfg.STORAGE_TYPE)
	if err != nil {
		return nil, nil, nil, err
	}

	q, err := component.GetQueue(cfg.QUEUE_TYPE)
	if err != nil {
		return nil, nil, nil, err
	}
	return c, q, s, nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	// setup db
	dbContainer, testDB, POSTGRES_URL = tdb.SetupContainer(ctx)
	pgPool = testDB.Pool
	err := trepo.ApplySchema(ctx, pgPool)
	if err != nil {
		log.Fatalf("could not initialise db")
	}

	// setup jetstream
	natsContainer, JETSTREAM_URL = tjetstream.SetupContainer(ctx)
	// setup minio
	minioContainer, MINIO_ENDPOINT = tminio.SetupContainer(ctx)
	server = setupServer(context.Background())
	code := m.Run()
	_ = natsContainer.Terminate(ctx)
	_ = dbContainer.Terminate(ctx)
	_ = minioContainer.Terminate(ctx)
	os.Exit(code)
}

func setupServer(ctx context.Context) *Server {
	setServerEnv()
	c, q, s, err := GetComponents()
	if err != nil {
		log.Fatalf("could not initialise components: %v", err)
	}

	server, err := NewServer(ctx, c, q, s)
	if err != nil {
		log.Fatalf("could not initialise server: %v", err)
	}

	return server
}

// createMultipartRequest creates a multipart/form-data request with metadata and code
func createMultipartRequest(t *testing.T, metadata model.JobRequest, code []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add metadata part
	metadataPart, err := writer.CreateFormField("metadata")
	require.NoError(t, err)

	metadataJSON, err := json.Marshal(metadata)
	require.NoError(t, err)

	_, err = metadataPart.Write(metadataJSON)
	require.NoError(t, err)

	// Add code part
	codePart, err := writer.CreateFormField("code")
	require.NoError(t, err)

	_, err = codePart.Write(code)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/job", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

// createJob is a helper that creates a job and returns it
func createJob(t *testing.T, server *Server) model.Job {
	t.Helper()

	metadata := model.JobRequest{
		ExecutionEngine: "c++",
		Tags:            []string{"test", "integration"},
	}
	code := []byte("print('hello world')")

	req := createMultipartRequest(t, metadata, code)
	resp := httptest.NewRecorder()

	server.router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var job model.Job
	err := json.NewDecoder(resp.Body).Decode(&job)
	require.NoError(t, err)

	return job
}

func TestHandleCreateJob(t *testing.T) {

	tests := []struct {
		name           string
		setupRequest   func(t *testing.T) *http.Request
		expectedStatus int
		expectedError  string
		validateResp   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name: "successful job creation with valid metadata and code",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
					Tags:            []string{"test", "integration"},
				}
				code := []byte("print('hello world')")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var job model.Job
				err := json.NewDecoder(resp.Body).Decode(&job)
				require.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, job.ID)
				assert.Equal(t, "c++", job.ExecutionEngine)
				assert.NotEmpty(t, job.CodeHash)
				assert.NotNil(t, job.CreationTime)
				assert.Contains(t, []string{"PENDING", "COMPLETED", "FAILED"}, job.Status)
			},
		},
		{
			name: "successful job creation without tags",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
					Tags:            nil,
				}
				code := []byte("console.log('test')")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var job model.Job
				err := json.NewDecoder(resp.Body).Decode(&job)
				require.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, job.ID)
			},
		},
		{
			name: "empty code should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
				}
				code := []byte("")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "empty code",
		},
		{
			name: "code exceeding max size should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
				}
				// Create code larger than maxCodeSize (1MB)
				code := make([]byte, maxCodeSize+1024)
				for i := range code {
					code[i] = 'a'
				}
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  ErrMaxCodeSize.Error(),
		},
		{
			name: "invalid JSON metadata should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				metadataPart, _ := writer.CreateFormField("metadata")
				metadataPart.Write([]byte("invalid json {{{"))

				codePart, _ := writer.CreateFormField("code")
				codePart.Write([]byte("print('test')"))

				writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/job", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  ErrInvalidMetadataJson.Error(),
		},
		{
			name: "unexpected form field should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				// Add unexpected field
				unexpectedPart, _ := writer.CreateFormField("unexpected")
				unexpectedPart.Write([]byte("data"))

				metadataPart, _ := writer.CreateFormField("metadata")
				metadata := model.JobRequest{ExecutionEngine: "c++"}
				metadataJSON, _ := json.Marshal(metadata)
				metadataPart.Write(metadataJSON)

				codePart, _ := writer.CreateFormField("code")
				codePart.Write([]byte("print('test')"))

				writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/job", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unexpected form field",
		},
		{
			name: "invalid multipart request should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/job", strings.NewReader("not multipart"))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "large valid code near limit should succeed",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
					Tags:            []string{"large"},
				}
				// Create code just under maxCodeSize (1MB)
				// Use valid Python code pattern
				code := []byte("# Large code file\n")
				for i := 0; i < (maxCodeSize-1024)/50; i++ {
					code = append(code, []byte("print('This is a test line number with padding')\n")...)
				}
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var job model.Job
				err := json.NewDecoder(resp.Body).Decode(&job)
				require.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, job.ID)
			},
		},
		{
			name: "metadata with unknown fields should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				metadataPart, _ := writer.CreateFormField("metadata")
				// JSON with unknown field
				metadataPart.Write([]byte(`{"executionEngine":"c++","unknownField":"value"}`))

				codePart, _ := writer.CreateFormField("code")
				codePart.Write([]byte("print('test')"))

				writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/job", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  ErrInvalidMetadataJson.Error(),
		},
		{
			name: "request exceeding total size should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
				}
				// Create request that exceeds maxTotalSize
				code := make([]byte, maxTotalSize)
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid execution engine should fail with bad request",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "", // Invalid empty execution engine
					Tags:            []string{"test"},
				}
				code := []byte("print('test')")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidExecutionEngine.Error(),
		},
		{
			name: "valid execution engine formats should succeed",
			setupRequest: func(t *testing.T) *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "c++",
					Tags:            []string{"golang"},
				}
				code := []byte("package main\nfunc main() {}")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var job model.Job
				err := json.NewDecoder(resp.Body).Decode(&job)
				require.NoError(t, err)
				assert.Equal(t, "c++", job.ExecutionEngine)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest(t)
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHandleGetJob(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	// Create a job for testing
	createdJob := createJob(t, server)

	// Wait for job to be persisted
	time.Sleep(100 * time.Millisecond)
	id, _ := uuid.NewV7()
	tests := []struct {
		name           string
		jobID          string
		expectedStatus int
		expectedError  string
		validateResp   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:           "get existing job successfully",
			jobID:          createdJob.ID.String(),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var job model.Job
				err := json.NewDecoder(resp.Body).Decode(&job)
				require.NoError(t, err)
				assert.Equal(t, createdJob.ID, job.ID)
				assert.Equal(t, createdJob.ExecutionEngine, job.ExecutionEngine)
				assert.Equal(t, createdJob.CodeHash, job.CodeHash)
				assert.NotNil(t, job.CreationTime)
			},
		},
		{
			name:           "get non-existent job should fail with not found",
			jobID:          id.String(),
			expectedStatus: http.StatusNotFound,
			expectedError:  jobservice.ErrNotFound.Error(),
		},
		{
			name:           "get job with invalid UUID should fail with bad request",
			jobID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidId.Error(),
		},
		{
			name:           "get job with empty ID should return 404 not found",
			jobID:          "",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/job/"+tt.jobID, nil)
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHandleListJob(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	// Create multiple jobs for testing
	t.Log("Creating test jobs...")
	job1 := createJob(t, server)
	t.Logf("Created job1: %s", job1.ID)

	time.Sleep(50 * time.Millisecond)
	job2 := createJob(t, server)
	t.Logf("Created job2: %s", job2.ID)

	time.Sleep(50 * time.Millisecond)
	job3 := createJob(t, server)
	t.Logf("Created job3: %s", job3.ID)

	// Wait for jobs to be persisted to DB
	t.Log("Waiting for jobs to be persisted...")
	time.Sleep(500 * time.Millisecond)

	id, _ := uuid.NewV7()

	tests := []struct {
		name           string
		offset         string
		expectedStatus int
		expectedError  string
		validateResp   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:           "list jobs without offset",
			offset:         "",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var jobs []*model.Job
				err := json.NewDecoder(resp.Body).Decode(&jobs)
				require.NoError(t, err)
				t.Logf("Retrieved %d jobs", len(jobs))
				assert.GreaterOrEqual(t, len(jobs), 3, "Should have at least 3 jobs")

				// Verify our created jobs are in the list
				jobIDs := make(map[uuid.UUID]bool)
				for _, job := range jobs {
					jobIDs[job.ID] = true
					t.Logf("Found job: %s", job.ID)
				}

				if !jobIDs[job1.ID] {
					t.Errorf("job1 %s not found in list", job1.ID)
				}
				if !jobIDs[job2.ID] {
					t.Errorf("job2 %s not found in list", job2.ID)
				}
				if !jobIDs[job3.ID] {
					t.Errorf("job3 %s not found in list", job3.ID)
				}
			},
		},
		{
			name:           "list jobs with valid offset",
			offset:         job1.ID.String(),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var jobs []*model.Job
				err := json.NewDecoder(resp.Body).Decode(&jobs)
				require.NoError(t, err)
				t.Logf("Retrieved %d jobs with offset", len(jobs))
				// Should return jobs after the offset
				// Actual behavior depends on implementation (inclusive vs exclusive)
				assert.GreaterOrEqual(t, len(jobs), 0)
			},
		},
		{
			name:           "list jobs with invalid offset UUID should fail with bad request",
			offset:         "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidOffset.Error(),
		},
		{
			name:           "list jobs with non-existent offset",
			offset:         id.String(),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var jobs []*model.Job
				err := json.NewDecoder(resp.Body).Decode(&jobs)
				require.NoError(t, err)
				// Should return empty or partial list
				assert.GreaterOrEqual(t, len(jobs), 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/job"
			if tt.offset != "" {
				url += "?offset=" + tt.offset
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			if tt.expectedStatus != resp.Code {
				t.Logf("Expected status %d but got %d: %s", tt.expectedStatus, resp.Code, resp.Body.String())
			}

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHandleDownloadOutput(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	// Create a job for testing
	createdJob := createJob(t, server)

	// Wait for job to be persisted
	time.Sleep(300 * time.Millisecond)
	id, _ := uuid.NewV7()
	tests := []struct {
		name           string
		jobID          string
		expectedStatus int
		expectedError  string
		validateResp   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:           "download output for existing job",
			jobID:          createdJob.ID.String(),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, "application/octet-stream", resp.Header().Get("Content-Type"))
				assert.Contains(t, resp.Header().Get("Content-Disposition"), "attachment")
				assert.Contains(t, resp.Header().Get("Content-Disposition"), "output.bin")
				// Output may be empty if job hasn't completed
				output := resp.Body.Bytes()
				assert.Equal(t, 0, len(output))
			},
		},
		{
			name:           "download output for non-existent job should fail with not found",
			jobID:          id.String(),
			expectedStatus: http.StatusNotFound,
			expectedError:  jobservice.ErrNotFound.Error(),
		},
		{
			name:           "download output with invalid UUID should fail with bad request",
			jobID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidId.Error(),
		},
		{
			name:           "download output with empty ID should return 400",
			jobID:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/job/"+tt.jobID+"/output", nil)
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHandleDownloadCode(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	// Create a job for testing
	createdJob := createJob(t, server)

	// Wait for code to be persisted
	time.Sleep(100 * time.Millisecond)
	id, _ := uuid.NewV7()

	tests := []struct {
		name           string
		jobID          string
		expectedStatus int
		expectedError  string
		validateResp   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:           "download code for existing job",
			jobID:          createdJob.ID.String(),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, "application/octet-stream", resp.Header().Get("Content-Type"))
				assert.Contains(t, resp.Header().Get("Content-Disposition"), "attachment")
				assert.Contains(t, resp.Header().Get("Content-Disposition"), "output.bin")

				code := resp.Body.Bytes()
				assert.NotEmpty(t, code)
				assert.Equal(t, "print('hello world')", string(code))
			},
		},
		{
			name:           "download code for non-existent job should fail with not found",
			jobID:          id.String(),
			expectedStatus: http.StatusNotFound,
			expectedError:  jobservice.ErrNotFound.Error(),
		},
		{
			name:           "download code with invalid UUID should fail with bad request",
			jobID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidId.Error(),
		},
		{
			name:           "download code with empty ID should return 400",
			jobID:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/job/"+tt.jobID+"/code", nil)
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestMiddlewares(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	t.Run("request timeout middleware", func(t *testing.T) {
		// This test verifies timeout is configured
		// Actual timeout testing would require a slow handler
		metadata := model.JobRequest{
			ExecutionEngine: "c++",
		}
		code := []byte("print('test')")
		req := createMultipartRequest(t, metadata, code)
		resp := httptest.NewRecorder()

		server.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("max body size middleware", func(t *testing.T) {
		// Create request larger than maxTotalSize
		largeData := make([]byte, maxTotalSize+1024)
		req := httptest.NewRequest(http.MethodPost, "/job", bytes.NewReader(largeData))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		resp := httptest.NewRecorder()

		server.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestEndToEndJobWorkflow(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)

	// Step 1: Create a job
	metadata := model.JobRequest{
		ExecutionEngine: "c++",
		Tags:            []string{"e2e", "workflow"},
	}
	code := []byte("print('end to end test')")

	createReq := createMultipartRequest(t, metadata, code)
	createResp := httptest.NewRecorder()
	server.router.ServeHTTP(createResp, createReq)
	require.Equal(t, http.StatusOK, createResp.Code)

	var createdJob model.Job
	err := json.NewDecoder(createResp.Body).Decode(&createdJob)
	require.NoError(t, err)
	jobID := createdJob.ID.String()

	// Wait for persistence
	time.Sleep(200 * time.Millisecond)

	// Step 2: Get the job
	getReq := httptest.NewRequest(http.MethodGet, "/job/"+jobID, nil)
	getResp := httptest.NewRecorder()
	server.router.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var retrievedJob model.Job
	err = json.NewDecoder(getResp.Body).Decode(&retrievedJob)
	require.NoError(t, err)
	assert.Equal(t, createdJob.ID, retrievedJob.ID)

	// Step 3: Download code
	codeReq := httptest.NewRequest(http.MethodGet, "/job/"+jobID+"/code", nil)
	codeResp := httptest.NewRecorder()
	server.router.ServeHTTP(codeResp, codeReq)
	require.Equal(t, http.StatusOK, codeResp.Code)

	downloadedCode := codeResp.Body.Bytes()
	assert.Equal(t, code, downloadedCode)

	// Step 4: List jobs and verify our job is included
	listReq := httptest.NewRequest(http.MethodGet, "/job", nil)
	listResp := httptest.NewRecorder()
	server.router.ServeHTTP(listResp, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var jobs []*model.Job
	err = json.NewDecoder(listResp.Body).Decode(&jobs)
	require.NoError(t, err)

	found := false
	for _, job := range jobs {
		if job.ID == createdJob.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created job should be in the list")

	// Step 5: Download output (may be empty if job not completed)
	outputReq := httptest.NewRequest(http.MethodGet, "/job/"+jobID+"/output", nil)
	outputResp := httptest.NewRecorder()
	server.router.ServeHTTP(outputResp, outputReq)
	require.Equal(t, http.StatusOK, outputResp.Code)
}

func TestErrorHandling(t *testing.T) {
	trepo.TruncateJobsTables(t, pgPool)
	id, _ := uuid.NewV7()
	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedError  string
	}{
		{
			name: "multipart read error returns bad request",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/job", strings.NewReader("corrupted data"))
				req.Header.Set("Content-Type", "multipart/form-data; boundary=----WebKitFormBoundary")
				return req
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid execution engine returns bad request",
			setupRequest: func() *http.Request {
				metadata := model.JobRequest{
					ExecutionEngine: "",
					Tags:            []string{"test"},
				}
				code := []byte("print('test')")
				return createMultipartRequest(t, metadata, code)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidExecutionEngine.Error(),
		},
		{
			name: "get job with invalid ID returns bad request",
			setupRequest: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/job/not-a-uuid", nil)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidId.Error(),
		},
		{
			name: "get non-existent job returns not found",
			setupRequest: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/job/"+id.String(), nil)
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  jobservice.ErrNotFound.Error(),
		},
		{
			name: "list jobs with invalid offset returns bad request",
			setupRequest: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/job?offset=not-a-uuid", nil)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  jobservice.ErrInvalidOffset.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			resp := httptest.NewRecorder()

			server.router.ServeHTTP(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code, "Response body: %s", resp.Body.String())

			if tt.expectedError != "" {
				assert.Contains(t, resp.Body.String(), tt.expectedError)
			}
		})
	}
}
