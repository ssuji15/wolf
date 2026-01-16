//go:build integration
// +build integration

package jetstream

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"github.com/ssuji15/wolf/internal/component/jetstream"
	"github.com/ssuji15/wolf/internal/queue"
	tjetstream "github.com/ssuji15/wolf/tests/integration_test/infra/jetstream"
)

var (
	natsContainer testcontainers.Container
	JETSTREAM_URL string
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	natsContainer, JETSTREAM_URL = tjetstream.SetupContainer(ctx)
	code := m.Run()
	_ = natsContainer.Terminate(ctx)
	os.Exit(code)
}

func resetQueueSingleton() {
	jqc = nil
}

func setQueueEnv() {
	os.Setenv("JETSTREAM_URL", JETSTREAM_URL)
	os.Setenv("MAX_MESSAGES_JOB_QUEUE", "5")
}

func newClient(t *testing.T) *JetStreamQueueClient {
	t.Helper()

	q, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	client, ok := q.(*JetStreamQueueClient)
	require.True(t, ok)

	return client
}

func TestNewJetStreamQueueClient(t *testing.T) {
	tests := []struct {
		name      string
		unsetEnv  string
		expectErr bool
	}{
		{"unset JETSTREAM_URL fails", "JETSTREAM_URL", true},
		{"unset Max Message fail", "MAX_MESSAGES_JOB_QUEUE", true},
		{"create client successfully", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetQueueSingleton()
			jetstream.ResetJetStreamClient()
			setQueueEnv()

			if tt.unsetEnv != "" {
				os.Unsetenv(tt.unsetEnv)
			}
			client, err := NewJetStreamQueueClient()
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				client, ok := client.(*JetStreamQueueClient)
				require.True(t, ok)
				_, err := client.context.StreamInfo(string(queue.EventStream))
				require.NoError(t, err)
				_, err = client.context.ConsumerInfo(string(queue.EventStream), string(queue.WORKER_CONSUMER))
				require.NoError(t, err)
				_, err = client.context.ConsumerInfo(string(queue.EventStream), string(queue.JOB_DB_CONSUMER))
				require.NoError(t, err)
				_, err = client.context.ConsumerInfo(string(queue.EventStream), string(queue.JOB_CODE_CONSUMER))
				require.NoError(t, err)
			}

		})
	}
}

func TestJetStreamQueueClient_AddStream(t *testing.T) {
	resetQueueSingleton()
	jetstream.ResetJetStreamClient()
	setQueueEnv()

	client := newClient(t)

	tests := []struct {
		name           string
		stream         string
		subjects       []string
		maxMsgs        int
		expectErr      bool
		verifyBehavior func(t *testing.T)
	}{
		{
			name:      "empty stream name",
			stream:    "",
			subjects:  []string{"empty.>"},
			maxMsgs:   10,
			expectErr: true,
		},
		{
			name:      "maxMsgs zero is invalid",
			stream:    "ZERO_MAX_STREAM",
			subjects:  []string{"zero.>"},
			maxMsgs:   0,
			expectErr: true,
		},
		{
			name:      "maxMsgs negative is invalid",
			stream:    "NEG_MAX_STREAM",
			subjects:  []string{"neg.>"},
			maxMsgs:   -1,
			expectErr: true,
		},
		{
			name:     "stream config is applied correctly",
			stream:   "CONFIG_STREAM",
			subjects: []string{"config.>"},
			maxMsgs:  2,
			verifyBehavior: func(t *testing.T) {
				info, err := client.context.StreamInfo("CONFIG_STREAM")
				require.NoError(t, err)

				cfg := info.Config
				require.Equal(t, int64(2), cfg.MaxMsgs)
				require.Equal(t, nats.InterestPolicy, cfg.Retention)
				require.Equal(t, nats.DiscardNew, cfg.Discard)
			},
		},
		{
			name:     "discard new messages when max messages reached",
			stream:   "DISCARD_STREAM",
			subjects: []string{"discard.>"},
			maxMsgs:  1,
			verifyBehavior: func(t *testing.T) {
				consumer := "DISCARD_CONSUMER"

				err := client.AddConsumer(
					queue.QueueEvent("DISCARD_STREAM"),
					consumer,
					nil,
					5,
				)
				_, err = client.context.Publish("discard.test", []byte("msg-1"))
				require.NoError(t, err)

				_, err = client.context.Publish("discard.test", []byte("msg-2"))
				require.Error(t, err)
			},
		},
		{
			name:     "interest policy drops messages without consumers",
			stream:   "INTEREST_STREAM",
			subjects: []string{"interest.>"},
			maxMsgs:  10,
			verifyBehavior: func(t *testing.T) {
				_, err := client.context.Publish("interest.test", []byte("msg-1"))
				require.NoError(t, err)

				info, err := client.context.StreamInfo("INTEREST_STREAM")
				require.NoError(t, err)
				require.Equal(t, uint64(0), info.State.Msgs)
			},
		},
		{
			name:     "happy path publish fetch with maxMsgs 2",
			stream:   "HP_STREAM",
			subjects: []string{"hp.>"},
			maxMsgs:  2,
			verifyBehavior: func(t *testing.T) {
				consumer := "HP_CONSUMER"

				err := client.AddConsumer(
					queue.QueueEvent("HP_STREAM"),
					consumer,
					nil,
					5,
				)
				require.NoError(t, err)

				sub, err := client.SubscribeEvent("hp.test", consumer)
				require.NoError(t, err)

				for i := 0; i < 5; i++ {
					m := fmt.Sprintf("msg-%d", i)
					err = client.PublishEvent(context.Background(), "hp.test", m)
					require.NoError(t, err)

					msgs, err := sub.Fetch(1, 2*time.Second)
					require.NoError(t, err)
					require.Len(t, msgs, 1)
					require.Equal(t, []byte(m), msgs[0].Data())
					require.NotNil(t, msgs[0].PublishedAt())
					require.NotNil(t, msgs[0].Ctx())
					require.Equal(t, 1, msgs[0].RetryCount())
					require.NoError(t, msgs[0].Ack())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AddStream(tt.stream, tt.subjects, tt.maxMsgs)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.verifyBehavior != nil {
				tt.verifyBehavior(t)
			}

			if tt.stream != "" {
				_ = client.context.DeleteStream(tt.stream)
			}
		})
	}
}

func TestJetStreamQueueClient_AddConsumer(t *testing.T) {
	client := newClient(t)

	err := client.AddStream("CONSUMER_STREAM", []string{"consumer.>"}, 100)
	require.NoError(t, err)

	tests := []struct {
		name       string
		consumer   string
		maxDeliver int
		wantErr    bool
	}{
		{"Add new consumer succeeds", "TEST_CONSUMER", 5, false},
		{"Add duplicate consumer again succeeds (no error)", "TEST_CONSUMER", 5, false},
		{"Empty consumer returns error", "", 5, true},
		{"invalid maxDeliver returns error", "BAD_CONSUMER", -5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AddConsumer(
				queue.QueueEvent("CONSUMER_STREAM"),
				tt.consumer,
				[]time.Duration{time.Second},
				tt.maxDeliver,
			)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJetStreamQueueClient_PublishEvent(t *testing.T) {
	client := newClient(t)

	err := client.AddStream("PUBLISH_STREAM", []string{"publish.>"}, 100)
	require.NoError(t, err)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"Publish valid event", "job-123", false},
		{"Publish empty ID fails", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.PublishEvent(context.Background(), "publish.test", tt.id)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestJetStreamQueueClient_SubscribeEvent(t *testing.T) {
	client := newClient(t)

	// Setup valid stream & consumer
	err := client.AddStream("SUB_STREAM", []string{"sub.>"}, 100)
	require.NoError(t, err)

	err = client.AddConsumer("SUB_STREAM", "SUB_CONSUMER", nil, 5)
	require.NoError(t, err)

	// Publish one message for success path
	err = client.PublishEvent(context.Background(), "sub.test", "hello")
	require.NoError(t, err)

	tests := []struct {
		name           string
		stream         queue.QueueEvent
		consumer       string
		fetchCount     int
		timeout        time.Duration
		expectSubErr   bool
		expectFetchErr bool
		expectMsgs     int
	}{
		{
			name:       "successfully fetch one message",
			stream:     "sub.test",
			consumer:   "SUB_CONSUMER",
			fetchCount: 1,
			timeout:    5 * time.Second,
			expectMsgs: 1,
		},
		{
			name:         "subscribe to unknown stream",
			stream:       "unknown.test",
			consumer:     "SUB_CONSUMER",
			expectSubErr: true,
		},
		{
			name:           "fetch when no data exists times out",
			stream:         "sub.test",
			consumer:       "SUB_CONSUMER",
			fetchCount:     1,
			timeout:        100 * time.Millisecond,
			expectFetchErr: true,
		},
		{
			name:           "fetch with negative count",
			stream:         "sub.test",
			consumer:       "SUB_CONSUMER",
			fetchCount:     -1,
			timeout:        1 * time.Second,
			expectFetchErr: true,
		},
		{
			name:           "fetch with zero timeout",
			stream:         "sub.test",
			consumer:       "SUB_CONSUMER",
			fetchCount:     1,
			timeout:        0,
			expectFetchErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, err := client.SubscribeEvent(tt.stream, tt.consumer)

			if tt.expectSubErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, sub)

			msgs, err := sub.Fetch(tt.fetchCount, tt.timeout)
			if tt.expectFetchErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, msgs, tt.expectMsgs)

			if tt.expectMsgs > 0 {
				require.Equal(t, []byte("hello"), msgs[0].Data())
				require.NoError(t, msgs[0].Term())
			}
		})
	}
}

func TestJetStreamQueueClient_Close(t *testing.T) {
	client := newClient(t)

	tests := []struct {
		name string
	}{
		{"close connection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Close()
			require.NoError(t, err)
		})
	}
}

func TestJetStreamQueueClient_ShutDown(t *testing.T) {
	client := newClient(t)

	tests := []struct {
		name string
	}{
		{"shutdown gracefully"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			client.ShutDown(ctx)
		})
	}
}
