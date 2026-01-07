package jetstream

import (
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/service/logger"
)

var (
	nc        *nats.Conn
	once      sync.Once
	initError error
)

func NewJetStreamClient() (*nats.Conn, error) {
	once.Do(func() {
		config, err := config.GetNatsConfig()
		if err != nil {
			initError = err
			return
		}
		nc, err = nats.Connect(config.URL,
			nats.MaxReconnects(-1),            // infinite retries
			nats.ReconnectWait(1*time.Second), // backoff
			nats.Name("wolf"),
			nats.ReconnectErrHandler(func(nc *nats.Conn, err error) {
				logger.Log.Error().Err(err).Msg("NATs reconnected")
			}),
			nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
				logger.Log.Error().Err(err).Msg("NATs disconnected")
			}),
			nats.ClosedHandler(func(nc *nats.Conn) {
				logger.Log.Error().Msg("NATs closed")
			}),
		)
		if err != nil {
			initError = err
			return
		}
	})
	return nc, initError
}

func ResetJetStreamClient() {
	nc = nil
	once = sync.Once{}
	initError = nil
}
