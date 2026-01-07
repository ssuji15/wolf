//go:build integration
// +build integration

package jetstream

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ssuji15/wolf/internal/component/jetstream"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	natsContainer testcontainers.Container
	JETSTREAM_URL string
)

// ------------------------
// TestMain - start NATS container
// ------------------------
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		fmt.Println("skipping integration tests")
		os.Exit(0)
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "nats:latest",
		ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		Cmd:          []string{"-js"},
		WaitingFor: wait.ForLog("Listening for client connections").
			WithStartupTimeout(30 * time.Second),
	}

	var err error
	natsContainer, err = testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
	if err != nil {
		panic(err)
	}

	host, err := natsContainer.Host(ctx)
	if err != nil {
		panic(err)
	}

	port, err := natsContainer.MappedPort(ctx, "4222")
	if err != nil {
		panic(err)
	}

	JETSTREAM_URL = fmt.Sprintf("nats://%s:%s", host, port.Port())

	code := m.Run()

	_ = natsContainer.Terminate(ctx)
	os.Exit(code)
}

// ------------------------
// Singleton reset helper
// ------------------------
func resetQueueSingleton() {
	jqc = nil
	initError = nil
	once = sync.Once{}
}

// ------------------------
// Env setup
// ------------------------
func setQueueEnv() {
	os.Setenv("JETSTREAM_URL", JETSTREAM_URL)
	os.Setenv("MAX_MESSAGES_JOB_QUEUE", "100")
}

// ------------------------
// 1. NewJetStreamQueueClient
// ------------------------
func TestNewJetStreamQueueClient(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	tests := []struct {
		name      string
		unsetEnv  string
		expectErr bool
	}{
		{"All env set succeeds", "", false},
		{"Missing JETSTREAM_URL fails", "JETSTREAM_URL", true},
		{"Missing MAX_MESSAGES_JOB_QUEUE fails", "MAX_MESSAGES_JOB_QUEUE", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resetQueueSingleton()
			setQueueEnv()
			jetstream.ResetJetStreamClient()
			if tt.unsetEnv != "" {
				os.Unsetenv(tt.unsetEnv)
			}

			c, err := NewJetStreamQueueClient()
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, c)
			} else {
				require.NoError(t, err)
				require.NotNil(t, c)
			}
		})
	}
}

// ------------------------
// 2. AddConsumer
// ------------------------
func TestJetStreamQueueClient_AddConsumer(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	tests := []struct {
		name       string
		stream     queue.QueueEvent
		consumer   string
		backoff    []time.Duration
		maxDeliver int
		expectErr  bool
	}{
		{"Add new consumer succeeds", queue.EventStream, "consumer1", []time.Duration{time.Second}, 5, false},
		{"Add same consumer again succeeds (no error)", queue.EventStream, "consumer1", []time.Duration{time.Second}, 5, false},
		{"Empty consumer returns error", queue.EventStream, "", []time.Duration{}, 5, true},
		{"Negative maxDeliver returns error", queue.EventStream, "consumer1", []time.Duration{}, -1, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := c.AddConsumer(tt.stream, tt.consumer, tt.backoff, tt.maxDeliver)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ------------------------
// 3. PublishEvent
// ------------------------
func TestJetStreamQueueClient_PublishEvent(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	ctx := context.Background()
	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	tests := []struct {
		name      string
		event     queue.QueueEvent
		id        string
		expectErr bool
	}{
		{"Publish valid event", queue.JobCreated, "id1", false},
		{"Publish empty ID fails", queue.EventStream, "", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := c.PublishEvent(ctx, tt.event, tt.id)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ------------------------
// 4. SubscribeEvent + Fetch
// ------------------------
func TestJetStreamQueueClient_SubscribeEventAndFetch(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	ctx := context.Background()
	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	jsClient := c.(*JetStreamQueueClient)

	consumerName := "consumer-fetch"
	err = jsClient.AddConsumer(queue.EventStream, consumerName, nil, 10)
	require.NoError(t, err)

	ids := []string{"id1", "id2", "id3"}
	for _, id := range ids {
		require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, id))
	}

	sub, err := c.SubscribeEvent(queue.JobCreated, consumerName)
	require.NoError(t, err)
	require.NotNil(t, sub)

	tests := []struct {
		name      string
		count     int
		timeout   time.Duration
		expectLen int
		expectErr bool
	}{
		{"Fetch 1 message", 1, 1 * time.Second, 1, false},
		{"Fetch remaining 2 messages", 3, 1 * time.Second, 2, false},
		{"Fetch with no messages, timesout", 1, 1 * time.Second, 0, true},
		{"Fetch with negative count, error", -1, 1 * time.Second, 0, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			msgs, err := sub.Fetch(ctx, tt.count, tt.timeout)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectLen, len(msgs))
			}
		})
	}
}

func TestJetStreamQueueClient_SubscribeEventAndFetchMessages(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	ctx := context.Background()
	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	jsClient := c.(*JetStreamQueueClient)

	consumerName := "consumer-fetch"
	err = jsClient.AddConsumer(queue.EventStream, consumerName, nil, 10)
	require.NoError(t, err)

	ids := []string{"id1", "id2", "id3"}
	for _, id := range ids {
		require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, id))
	}

	sub, err := c.SubscribeEvent(queue.JobCreated, consumerName)
	require.NoError(t, err)
	require.NotNil(t, sub)

	msgs, err := sub.Fetch(ctx, 3, 1*time.Second)
	require.NoError(t, err)
	require.Len(t, msgs, 3)

	for idx, qm := range msgs {
		// Type assertion to NatsData
		data, ok := qm.(*NatsData)
		require.True(t, ok)

		require.NotEmpty(t, data.Data())
		require.Contains(t, ids[idx], string(data.Data()))

		require.False(t, data.PublishedAt().IsZero())

		require.NotNil(t, data.Ctx())
		require.GreaterOrEqual(t, data.RetryCount(), 1)
		require.NoError(t, data.Ack())
	}

	require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "terminate-me"))
	count := 0
	for {
		msgs, err = sub.Fetch(ctx, 1, 1*time.Second)
		if err != nil || count > 1 {
			break
		}

		data := msgs[0].(*NatsData)
		require.NoError(t, data.Term())
		count++
	}
	require.Equal(t, count, 1)
}

func TestJetStreamQueueClient_MaxDeliver(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	ctx := context.Background()
	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	jsClient := c.(*JetStreamQueueClient)

	// -------------------------
	// 1. Create consumer with MaxDeliver = 3
	// -------------------------
	const maxDeliver = 3
	consumerName := "consumer-max-deliver"

	err = jsClient.AddConsumer(
		queue.EventStream,
		consumerName,
		[]time.Duration{500 * time.Millisecond},
		maxDeliver,
	)
	require.NoError(t, err)

	// -------------------------
	// 2. Publish one message
	// -------------------------
	id := "retry-test"
	require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, id))

	sub, err := c.SubscribeEvent(queue.JobCreated, consumerName)
	require.NoError(t, err)

	// -------------------------
	// 3. Fetch & timeout message repeatedly
	// -------------------------
	deliveries := 0
	var lastRetryCount int

	for {
		msgs, err := sub.Fetch(ctx, 1, 1*time.Second)
		if err != nil {
			// No more messages expected after maxDeliver
			break
		}
		if len(msgs) == 0 {
			break
		}

		msg := msgs[0].(*NatsData)
		if string(msg.Data()) == id {
			deliveries++
			lastRetryCount = msg.RetryCount()
		}
	}

	// -------------------------
	// 4. Assertions
	// -------------------------
	require.Equal(t, maxDeliver, deliveries)
	require.Equal(t, maxDeliver, lastRetryCount)
}

// ------------------------
// 5. ShutDown
// ------------------------
func TestJetStreamQueueClient_ShutDown(t *testing.T) {
	resetQueueSingleton()
	setQueueEnv()

	ctx := context.Background()
	c, err := NewJetStreamQueueClient()
	require.NoError(t, err)

	jsClient := c.(*JetStreamQueueClient)

	consumerName := "consumer-fetch"
	err = jsClient.AddConsumer(queue.EventStream, consumerName, nil, 10)
	require.NoError(t, err)

	// pre-publish
	require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "shutdown-test"))

	c.ShutDown(ctx)

	// after shutdown, further publish should fail
	err = c.PublishEvent(ctx, queue.JobCreated, "fail-after-shutdown")
	require.Error(t, err)
}

// func TestJetStreamQueueClient_StreamPolicies(t *testing.T) {
// 	tests := []struct {
// 		name string
// 		run  func(t *testing.T)
// 	}{
// 		{
// 			name: "InterestPolicy drops messages published before consumer exists",
// 			run: func(t *testing.T) {
// 				resetQueueSingleton()
// 				setQueueEnv()
// 				os.Setenv("MAX_MESSAGES_JOB_QUEUE", "10")

// 				ctx := context.Background()
// 				c, err := NewJetStreamQueueClient()
// 				require.NoError(t, err)

// 				// Publish BEFORE consumer creation
// 				require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "pre-1"))
// 				require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "pre-2"))

// 				jsClient := c.(*JetStreamQueueClient)
// 				consumer := "interest-consumer"

// 				require.NoError(t,
// 					jsClient.AddConsumer(queue.EventStream, consumer, nil, 5),
// 				)

// 				sub, err := c.SubscribeEvent(queue.JobCreated, consumer)
// 				require.NoError(t, err)

// 				// Should receive nothing
// 				msgs, err := sub.Fetch(ctx, 1, 500*time.Millisecond)
// 				require.Error(t, err)
// 				require.Len(t, msgs, 0)
// 			},
// 		},

// 		{
// 			name: "MaxMsgs with DiscardNew rejects new messages",
// 			run: func(t *testing.T) {
// 				resetQueueSingleton()
// 				setQueueEnv()
// 				os.Setenv("MAX_MESSAGES_JOB_QUEUE", "2")

// 				ctx := context.Background()
// 				c, err := NewJetStreamQueueClient()
// 				require.NoError(t, err)

// 				jsClient := c.(*JetStreamQueueClient)
// 				consumer := "discard-consumer"

// 				require.NoError(t,
// 					jsClient.AddConsumer(queue.EventStream, consumer, nil, 5),
// 				)

// 				sub, err := c.SubscribeEvent(queue.JobCreated, consumer)
// 				require.NoError(t, err)

// 				require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "msg-1"))
// 				require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "msg-2"))

// 				// Exceeds MaxMsgs → DiscardNew
// 				err = c.PublishEvent(ctx, queue.JobCreated, "msg-3")
// 				require.Error(t, err)

// 				msgs, err := sub.Fetch(ctx, 5, 1*time.Second)
// 				require.NoError(t, err)
// 				require.Len(t, msgs, 2)

// 				values := []string{
// 					string(msgs[0].Data()),
// 					string(msgs[1].Data()),
// 				}
// 				require.ElementsMatch(t, []string{"msg-1", "msg-2"}, values)
// 			},
// 		},

// 		{
// 			name: "MaxMsgs does not remove existing messages",
// 			run: func(t *testing.T) {
// 				resetQueueSingleton()
// 				setQueueEnv()
// 				os.Setenv("MAX_MESSAGES_JOB_QUEUE", "1")

// 				ctx := context.Background()
// 				c, err := NewJetStreamQueueClient()
// 				require.NoError(t, err)

// 				jsClient := c.(*JetStreamQueueClient)
// 				consumer := "maxmsg-consumer"

// 				require.NoError(t,
// 					jsClient.AddConsumer(queue.EventStream, consumer, nil, 5),
// 				)

// 				sub, err := c.SubscribeEvent(queue.JobCreated, consumer)
// 				require.NoError(t, err)

// 				require.NoError(t, c.PublishEvent(ctx, queue.JobCreated, "only-msg"))

// 				msgs, err := sub.Fetch(ctx, 1, 1*time.Second)
// 				require.NoError(t, err)
// 				require.Len(t, msgs, 1)
// 				require.Equal(t, "only-msg", string(msgs[0].Data()))

// 				// Second publish should fail
// 				err = c.PublishEvent(ctx, queue.JobCreated, "should-fail")
// 				require.Error(t, err)
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			tt.run(t)
// 		})
// 	}
// }

// func deleteStreamIfExist(js nats.JetStreamContext) error {
// 	return js.DeleteStream(string(queue.EventStream))
// }
