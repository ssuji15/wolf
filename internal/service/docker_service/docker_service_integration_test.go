package dockerservice

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/model"
)

var (
	testImage = "nginx:latest"
)

// setupDockerTest initializes Docker client and pulls test image
func setupDockerTest(t *testing.T) (*DockerService, string) {
	t.Helper()

	// Verify Docker is available
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	require.NoError(t, err, "Docker client initialization failed")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx, client.PingOptions{})
	require.NoError(t, err, "Docker daemon is not accessible")

	// Pull test image
	pullTestImage(t, cli)

	// Initialize DockerService
	cfg := &config.SandboxManagerConfig{}
	dockerService, err := NewDockerService(cfg)
	require.NoError(t, err, "Failed to initialize DockerService")

	// Create temp directory for mounts
	tempDir, err := os.MkdirTemp("", "docker-integration-test-*")
	require.NoError(t, err, "Failed to create temp directory")

	// Cleanup function
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
		cli.Close()
	})

	return dockerService, tempDir
}

func pullTestImage(t *testing.T, cli *client.Client) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reader, err := cli.ImagePull(ctx, testImage, client.ImagePullOptions{})
	require.NoError(t, err, "Failed to pull test image")
	defer reader.Close()

	// Wait for pull to complete
	_, err = io.Copy(io.Discard, reader)
	require.NoError(t, err, "Failed to complete image pull")
}

func createWorkDir(t *testing.T, baseDir, name string) string {
	t.Helper()

	workDir := filepath.Join(baseDir, name)
	err := os.MkdirAll(workDir, 0755)
	require.NoError(t, err, "Failed to create work directory")
	return workDir
}

func cleanupContainer(t *testing.T, dockerService *DockerService, containerID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := dockerService.RemoveContainer(ctx, containerID)
	require.NoError(t, err)
}

func TestCreateContainer(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name           string
		opts           model.CreateOptions
		seccompProfile string
		wantErr        bool
		errContains    string
		validate       func(*testing.T, model.WorkerMetadata)
		timeout        time.Duration
	}{
		{
			name: "successful container creation with basic config",
			opts: model.CreateOptions{
				Name:            "test-container-basic",
				Image:           testImage,
				WorkDir:         createWorkDir(t, tempDir, "basic"),
				AppArmorProfile: "unconfined",
				CPUQuota:        100000,
				MemoryLimit:     67108864, // 64MB
				Labels: map[string]string{
					"test": "integration",
					"type": "basic",
				},
			},
			seccompProfile: "",
			wantErr:        false,
			validate: func(t *testing.T, meta model.WorkerMetadata) {
				assert.NotEmpty(t, meta.ID, "Container ID should not be empty")
				assert.Equal(t, "test-container-basic", meta.Name)
				assert.Equal(t, "created", meta.Status)
				assert.False(t, meta.CreatedAt.IsZero())
				assert.False(t, meta.UpdatedAt.IsZero())
			},
			timeout: 30 * time.Second,
		},
		{
			name: "container creation with custom labels",
			opts: model.CreateOptions{
				Name:            "test-container-labels",
				Image:           testImage,
				WorkDir:         createWorkDir(t, tempDir, "labels"),
				AppArmorProfile: "unconfined",
				CPUQuota:        50000,
				MemoryLimit:     33554432, // 32MB
				Labels: map[string]string{
					"environment": "test",
					"version":     "1.0.0",
					"team":        "platform",
				},
			},
			seccompProfile: "",
			wantErr:        false,
			validate: func(t *testing.T, meta model.WorkerMetadata) {
				assert.NotEmpty(t, meta.ID)
				assert.Equal(t, "test-container-labels", meta.Name)
			},
			timeout: 30 * time.Second,
		},
		{
			name: "container creation with resource limits",
			opts: model.CreateOptions{
				Name:            "test-container-resources",
				Image:           testImage,
				WorkDir:         createWorkDir(t, tempDir, "resources"),
				AppArmorProfile: "unconfined",
				CPUQuota:        200000,
				MemoryLimit:     134217728, // 128MB
				Labels: map[string]string{
					"test": "resources",
				},
			},
			seccompProfile: "",
			wantErr:        false,
			validate: func(t *testing.T, meta model.WorkerMetadata) {
				assert.NotEmpty(t, meta.ID)
			},
			timeout: 30 * time.Second,
		},
		{
			name: "container creation with minimal config",
			opts: model.CreateOptions{
				Name:            "test-container-minimal",
				Image:           testImage,
				WorkDir:         createWorkDir(t, tempDir, "minimal"),
				AppArmorProfile: "unconfined",
				CPUQuota:        100000,
				MemoryLimit:     67108864,
				Labels:          map[string]string{},
			},
			seccompProfile: "",
			wantErr:        false,
			validate: func(t *testing.T, meta model.WorkerMetadata) {
				assert.NotEmpty(t, meta.ID)
				assert.Equal(t, "test-container-minimal", meta.Name)
			},
			timeout: 30 * time.Second,
		},
		{
			name: "container creation with invalid image",
			opts: model.CreateOptions{
				Name:            "test-container-invalid",
				Image:           "nonexistent-image:invalid-tag-12345",
				WorkDir:         createWorkDir(t, tempDir, "invalid"),
				AppArmorProfile: "unconfined",
				CPUQuota:        100000,
				MemoryLimit:     67108864,
				Labels: map[string]string{
					"test": "invalid",
				},
			},
			seccompProfile: "",
			wantErr:        true,
			timeout:        30 * time.Second,
		},
		{
			name: "container creation with empty apparmor profile",
			opts: model.CreateOptions{
				Name:            "test-container-no-apparmor",
				Image:           testImage,
				WorkDir:         createWorkDir(t, tempDir, "no-apparmor"),
				AppArmorProfile: "",
				CPUQuota:        100000,
				MemoryLimit:     67108864,
				Labels:          map[string]string{"test": "apparmor"},
			},
			seccompProfile: "",
			wantErr:        false,
			validate: func(t *testing.T, meta model.WorkerMetadata) {
				assert.NotEmpty(t, meta.ID)
			},
			timeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			meta, err := dockerService.CreateContainer(ctx, tt.opts, tt.seccompProfile)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				if tt.validate != nil {
					tt.validate(t, meta)
				}
				return
			}

			require.NoError(t, err)
			t.Cleanup(func() {
				cleanupContainer(t, dockerService, meta.ID)
			})

			if tt.validate != nil {
				tt.validate(t, meta)
			}

			// Verify container exists and is running
			inspectResult, err := dockerService.InspectContainer(ctx, meta.ID)
			require.NoError(t, err)
			assert.True(t, inspectResult.Container.State.Running, "Container should be running")

			// Verify image
			assert.Equal(t, tt.opts.Image, inspectResult.Container.Config.Image)

			// Verify labels
			for key, value := range tt.opts.Labels {
				assert.Equal(t, value, inspectResult.Container.Config.Labels[key])
			}

			// Verify mounts
			require.Len(t, inspectResult.Container.Mounts, 1)
			assert.Equal(t, tt.opts.WorkDir, inspectResult.Container.Mounts[0].Source)
			assert.Equal(t, "/job", inspectResult.Container.Mounts[0].Destination)
		})
	}
}

func TestStopContainer(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name        string
		setupFunc   func(context.Context) string
		wantErr     bool
		errContains string
		validate    func(*testing.T, string)
	}{
		{
			name: "stop running container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-stop-running",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "stop-running"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "stop"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})
				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, containerID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				inspectResult, err := dockerService.InspectContainer(ctx, containerID)
				require.NoError(t, err)
				assert.False(t, inspectResult.Container.State.Running, "Container should be stopped")
			},
		},
		{
			name: "stop already stopped container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-stop-already-stopped",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "stop-already"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "stop"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})

				// Stop it once
				_, err = dockerService.StopContainer(ctx, meta.ID)
				require.NoError(t, err)

				return meta.ID
			},
			wantErr: false, // Docker allows stopping already stopped containers
		},
		{
			name: "stop non-existent container",
			setupFunc: func(ctx context.Context) string {
				return "non-existent-container-id-12345"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			containerID := tt.setupFunc(ctx)

			result, err := dockerService.StopContainer(ctx, containerID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)

			if tt.validate != nil {
				tt.validate(t, containerID)
			}
		})
	}
}

func TestRestartContainer(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name        string
		setupFunc   func(context.Context) string
		wantErr     bool
		errContains string
		validate    func(*testing.T, string)
	}{
		{
			name: "restart running container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-restart-running",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "restart-running"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "restart"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})
				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, containerID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				inspectResult, err := dockerService.InspectContainer(ctx, containerID)
				require.NoError(t, err)
				assert.True(t, inspectResult.Container.State.Running, "Container should be running after restart")
			},
		},
		{
			name: "restart stopped container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-restart-stopped",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "restart-stopped"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "restart"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})

				_, err = dockerService.StopContainer(ctx, meta.ID)
				require.NoError(t, err)

				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, containerID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				inspectResult, err := dockerService.InspectContainer(ctx, containerID)
				require.NoError(t, err)
				assert.True(t, inspectResult.Container.State.Running, "Container should be running after restart")
			},
		},
		{
			name: "restart non-existent container",
			setupFunc: func(ctx context.Context) string {
				return "non-existent-restart-id"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			containerID := tt.setupFunc(ctx)

			result, err := dockerService.RestartContainer(ctx, containerID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)

			if tt.validate != nil {
				tt.validate(t, containerID)
			}
		})
	}
}

func TestRemoveContainer(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name        string
		setupFunc   func(context.Context) string
		wantErr     bool
		errContains string
		validate    func(*testing.T, string)
	}{
		{
			name: "remove stopped container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-remove-stopped",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "remove-stopped"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "remove"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)

				_, err = dockerService.StopContainer(ctx, meta.ID)
				require.NoError(t, err)

				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, containerID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				_, err := dockerService.InspectContainer(ctx, containerID)
				assert.Error(t, err, "Container should not exist after removal")
			},
		},
		{
			name: "force remove running container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-remove-running",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "remove-running"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "remove"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, containerID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				_, err := dockerService.InspectContainer(ctx, containerID)
				assert.Error(t, err, "Container should not exist after removal")
			},
		},
		{
			name: "remove non-existent container",
			setupFunc: func(ctx context.Context) string {
				return "non-existent-remove-id"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			containerID := tt.setupFunc(ctx)

			result, err := dockerService.RemoveContainer(ctx, containerID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)

			if tt.validate != nil {
				tt.validate(t, containerID)
			}
		})
	}
}

func TestInspectContainer(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name        string
		setupFunc   func(context.Context) string
		wantErr     bool
		errContains string
		validate    func(*testing.T, client.ContainerInspectResult)
	}{
		{
			name: "inspect running container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-inspect-running",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "inspect-running"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels: map[string]string{
						"test":        "inspect",
						"environment": "integration",
					},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})
				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, result client.ContainerInspectResult) {
				assert.NotNil(t, result.Container.State)
				assert.True(t, result.Container.State.Running)
				assert.Equal(t, "inspect", result.Container.Config.Labels["test"])
				assert.Equal(t, "integration", result.Container.Config.Labels["environment"])
				assert.NotEmpty(t, result.Container.ID)
			},
		},
		{
			name: "inspect stopped container",
			setupFunc: func(ctx context.Context) string {
				opts := model.CreateOptions{
					Name:            "test-inspect-stopped",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "inspect-stopped"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "inspect-stopped"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanupContainer(t, dockerService, meta.ID)
				})

				_, err = dockerService.StopContainer(ctx, meta.ID)
				require.NoError(t, err)

				return meta.ID
			},
			wantErr: false,
			validate: func(t *testing.T, result client.ContainerInspectResult) {
				assert.NotNil(t, result.Container.State)
				assert.False(t, result.Container.State.Running)
				assert.Equal(t, "inspect-stopped", result.Container.Config.Labels["test"])
			},
		},
		{
			name: "inspect non-existent container",
			setupFunc: func(ctx context.Context) string {
				return "non-existent-inspect-id"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			containerID := tt.setupFunc(ctx)

			result, err := dockerService.InspectContainer(ctx, containerID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestContainerWait(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	tests := []struct {
		name      string
		setupFunc func(context.Context) (string, func())
		condition container.WaitCondition
		timeout   time.Duration
		validate  func(*testing.T, client.ContainerWaitResult, error)
	}{
		{
			name: "wait for container removal",
			setupFunc: func(ctx context.Context) (string, func()) {
				opts := model.CreateOptions{
					Name:            "test-wait-remove",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "wait-remove"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "wait"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)

				// Remove container after a short delay
				go func() {
					time.Sleep(2 * time.Second)
					cleanupContainer(t, dockerService, meta.ID)
				}()

				return meta.ID, func() {}
			},
			condition: container.WaitConditionRemoved,
			timeout:   10 * time.Second,
			validate: func(t *testing.T, result client.ContainerWaitResult, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			},
		},
		{
			name: "wait for container to stop",
			setupFunc: func(ctx context.Context) (string, func()) {
				opts := model.CreateOptions{
					Name:            "test-wait-stop",
					Image:           testImage,
					WorkDir:         createWorkDir(t, tempDir, "wait-stop"),
					AppArmorProfile: "unconfined",
					CPUQuota:        100000,
					MemoryLimit:     67108864,
					Labels:          map[string]string{"test": "wait"},
				}
				meta, err := dockerService.CreateContainer(ctx, opts, "")
				require.NoError(t, err)

				cleanup := func() {
					cleanupContainer(t, dockerService, meta.ID)
				}

				// Stop container after a short delay
				go func() {
					time.Sleep(2 * time.Second)
					_, _ = dockerService.StopContainer(ctx, meta.ID)
				}()

				return meta.ID, cleanup
			},
			condition: container.WaitConditionNotRunning,
			timeout:   10 * time.Second,
			validate: func(t *testing.T, result client.ContainerWaitResult, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			containerID, cleanup := tt.setupFunc(ctx)
			if cleanup != nil {
				t.Cleanup(cleanup)
			}

			result := dockerService.ContainerWait(ctx, containerID, tt.condition)

			if tt.validate != nil {
				tt.validate(t, result, nil)
			}
		})
	}
}

func TestContainerLifecycle(t *testing.T) {
	dockerService, tempDir := setupDockerTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create
	opts := model.CreateOptions{
		Name:            "test-lifecycle-complete",
		Image:           testImage,
		WorkDir:         createWorkDir(t, tempDir, "lifecycle"),
		AppArmorProfile: "unconfined",
		CPUQuota:        100000,
		MemoryLimit:     67108864,
		Labels: map[string]string{
			"test": "lifecycle",
		},
	}

	meta, err := dockerService.CreateContainer(ctx, opts, "")
	require.NoError(t, err)

	assert.NotEmpty(t, meta.ID)
	assert.Equal(t, "test-lifecycle-complete", meta.Name)

	// Verify running
	inspectResult, err := dockerService.InspectContainer(ctx, meta.ID)
	require.NoError(t, err)
	assert.True(t, inspectResult.Container.State.Running)

	// Stop
	stopResult, err := dockerService.StopContainer(ctx, meta.ID)
	require.NoError(t, err)
	assert.NotNil(t, stopResult)

	inspectResult, err = dockerService.InspectContainer(ctx, meta.ID)
	require.NoError(t, err)
	assert.False(t, inspectResult.Container.State.Running)

	// Restart
	restartResult, err := dockerService.RestartContainer(ctx, meta.ID)
	require.NoError(t, err)
	assert.NotNil(t, restartResult)

	inspectResult, err = dockerService.InspectContainer(ctx, meta.ID)
	require.NoError(t, err)
	assert.True(t, inspectResult.Container.State.Running)

	// Stop again before removal
	_, err = dockerService.StopContainer(ctx, meta.ID)
	require.NoError(t, err)

	// Remove
	cleanupContainer(t, dockerService, meta.ID)

	// Verify removal
	_, err = dockerService.InspectContainer(ctx, meta.ID)
	assert.Error(t, err, "Container should not exist after removal")
}
