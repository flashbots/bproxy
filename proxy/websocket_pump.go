package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/utils"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type websocketPump struct {
	cfg *websocketConfig

	frontend, backend *websocket.Conn
	logger            *zap.Logger

	active atomic.Bool

	done chan struct{}
}

func newWebsocketPump(
	cfg *websocketConfig,
	frontend, backend *websocket.Conn,
	logger *zap.Logger,
) *websocketPump {
	return &websocketPump{
		cfg:      cfg,
		frontend: frontend,
		backend:  backend,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

func (w *websocketPump) run() error {
	failure := make(chan error, 2)
	done := make(chan struct{}, 1)

	go w.pump(w.backend, w.frontend, "b->f", done, failure)

	w.active.Store(true)

	for {
		select {
		case <-w.done:
			done <- struct{}{}
			w.active.Store(false)
			return nil
		case err := <-failure:
			done <- struct{}{}
			w.active.Store(false)
			return err
		}
	}
}

func (w *websocketPump) stop() error {
	w.done <- struct{}{}

	errs := make([]error, 0)

	countdown := 60
	for w.active.Load() && countdown > 0 {
		time.Sleep(time.Second)
		countdown--
	}
	if countdown == 0 {
		errs = append(errs, errors.New("timed out gracefully stopping the pump"))
	}

	if err := w.backend.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := w.frontend.Close(); err != nil {
		errs = append(errs, err)
	}

	return utils.FlattenErrors(errs)
}

func (w *websocketPump) pump(
	from, to *websocket.Conn,
	direction string,
	done chan struct{},
	failure chan error,
) {
	l := w.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)

loop:
	for {
		select {
		case <-done:
			return

		default:
			if err := from.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				failure <- err
				continue loop
			}
			msgType, bytes, err := from.ReadMessage()
			if err != nil {
				failure <- err
				continue loop
			}

			ts := time.Now()

			if err := to.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				failure <- err
				continue loop
			}
			err = to.WriteMessage(msgType, bytes)
			if err != nil {
				failure <- err
				continue loop
			}

			loggedFields := make([]zap.Field, 0, 6)
			loggedFields = append(loggedFields,
				zap.Time("ts_message_received", ts),
				zap.Int("message_size", len(bytes)),
			)

			if w.cfg.proxy.LogMessages && len(bytes) <= w.cfg.proxy.LogMessagesMaxSize {
				var jsonMessage interface{}
				if err := json.Unmarshal(bytes, &jsonMessage); err == nil {
					loggedFields = append(loggedFields,
						zap.Any("json_message", jsonMessage),
					)
				} else {
					loggedFields = append(loggedFields,
						zap.NamedError("error_unmarshal", err),
						zap.String("websocket_message", utils.Str(bytes)),
					)
				}
			}

			metrics.ProxySuccessCount.Add(context.TODO(), 1, otelapi.WithAttributes(
				attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(w.cfg.name)},
				attribute.KeyValue{Key: "direction", Value: attribute.StringValue(direction)},
			))
			l.Info("Proxied message", loggedFields...)
		}
	}
}
