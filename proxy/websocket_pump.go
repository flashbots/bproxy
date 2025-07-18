package proxy

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/flashbots/bproxy/flashblock"
	"github.com/flashbots/bproxy/jrpc"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/triaged"
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

func (p *websocketPump) run() error {
	failure := make(chan error, 2)
	done := make(chan struct{}, 2)

	p.frontend.SetPingHandler(p.pumpPings(p.frontend, p.backend, "f->b"))
	p.backend.SetPongHandler(p.pumpPongs(p.backend, p.frontend, "b->f"))

	p.backend.SetPingHandler(p.pumpPings(p.backend, p.frontend, "b->f"))
	p.frontend.SetPongHandler(p.pumpPongs(p.frontend, p.backend, "f->b"))

	p.frontend.SetCloseHandler(p.pumpCloseMessages(p.frontend, p.backend, "f->b"))
	p.backend.SetCloseHandler(p.pumpCloseMessages(p.backend, p.frontend, "b->f"))

	go p.pumpMessages(p.backend, p.frontend, p.cfg.proxy.ForwardTimeout, "b->f", done, failure)
	go p.pumpMessages(p.frontend, p.backend, p.cfg.proxy.BackwardTimeout, "f->b", done, failure)

	p.active.Store(true)

	for {
		select {
		case <-p.done:
			done <- struct{}{}
			p.active.Store(false)
			return nil

		case err := <-failure:
			errs := make([]error, 0, 2)
			errs = append(errs, err)

		exhaustErrs:
			for {
				select {
				case err := <-failure:
					errs = append(errs, err)
				default:
					break exhaustErrs
				}
			}
			err = utils.FlattenErrors(errs)

			done <- struct{}{}
			p.active.Store(false)
			return err
		}
	}
}

func (p *websocketPump) stop() error {
	p.done <- struct{}{}

	errs := make([]error, 0)

	countdown := 60
	for p.active.Load() && countdown > 0 {
		time.Sleep(time.Second)
		countdown--
	}
	if countdown == 0 {
		errs = append(errs, errors.New("timed out gracefully stopping the pump"))
	}

	if err := p.backend.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.frontend.Close(); err != nil {
		errs = append(errs, err)
	}

	return utils.FlattenErrors(errs)
}

func (p *websocketPump) pumpMessages(
	from, to *websocket.Conn,
	timeout time.Duration,
	direction string,
	done chan struct{},
	failure chan error,
) {
	l := p.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)

	messages := make(chan *websocketMessage, 16)
	doneReads := make(chan struct{})
	doneWrites := make(chan struct{})

	go func() { // read
		for {
			select {
			case <-doneReads:
				return

			default:
				if err := from.SetReadDeadline(utils.Deadline(timeout)); err != nil {
					failure <- err
					continue
				}

				msgType, bytes, err := from.ReadMessage()
				if err != nil {
					failure <- err
					continue
				}

				messages <- &websocketMessage{
					msgType: msgType,
					bytes:   bytes,
					ts:      time.Now(),
				}
			}
		}
	}()

	go func() { // write
		for {
			select {
			case <-doneWrites:
				return

			case m := <-messages:
				loggedFields := make([]zap.Field, 0, 6)

				if p.cfg.proxy.Chaos.Enabled { // inject chaos
					dropMessage := rand.Float64() < p.cfg.proxy.Chaos.DroppedMessageProbability/100
					injectInvalidFlashblockPayload := rand.Float64() < p.cfg.proxy.Chaos.InjectedInvalidFlashblockPayloadProbability/100
					injectMalformedJsonMessage := rand.Float64() < p.cfg.proxy.Chaos.InjectedMalformedJsonMessageProbability/100

					if dropMessage { // drop message
						l.Info("Dropped message", p.prepareLogFields(m,
							zap.Bool("chaos_dropped_message", true),
						)...)
						continue
					}

					if injectInvalidFlashblockPayload { // inject invalid flashblock payload
						if err := m.chaosMangle(); err == nil {
							loggedFields = append(loggedFields,
								zap.Bool("chaos_injected_invalid_flashblock_payload", true),
							)
						} else {
							l.Warn("Failed to generate invalid flashblock payload",
								zap.Error(err),
							)
						}
					}

					if injectMalformedJsonMessage { // inject malformed json
						loggedFields = append(loggedFields,
							zap.Bool("chaos_injected_malformed_json_message", true),
						)
						m.bytes = m.bytes[1 : len(m.bytes)-1]
					}

					{ // chaos-inject latency
						if p.cfg.proxy.Chaos.MinInjectedLatency > 0 || p.cfg.proxy.Chaos.MaxInjectedLatency > 0 {
							loggedFields = append(loggedFields,
								zap.Bool("chaos_injected_latency", true),
							)
							latency := time.Duration(rand.Int64N(int64(p.cfg.proxy.Chaos.MaxInjectedLatency) + 1))
							latency = max(latency, p.cfg.proxy.Chaos.MinInjectedLatency)
							time.Sleep(latency - time.Since(m.ts))
						}
					}
				}

				if err := to.SetWriteDeadline(utils.Deadline(timeout)); err != nil {
					failure <- err
					continue
				}

				if err := to.WriteMessage(m.msgType, m.bytes); err != nil {
					failure <- err
					continue
				}

				{ // emit logs and metrics
					metrics.ProxySuccessCount.Add(context.TODO(), 1, otelapi.WithAttributes(
						attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
						attribute.KeyValue{Key: "direction", Value: attribute.StringValue(direction)},
					))

					l.Info("Proxied message", p.prepareLogFields(m, loggedFields...)...)
				}
			}
		}
	}()

	go func() { // terminate
		<-done
		doneReads <- struct{}{}
		doneWrites <- struct{}{}
	}()
}

func (p *websocketPump) prepareLogFields(
	m *websocketMessage,
	extraLogFields ...zap.Field,
) []zap.Field {
	loggedFields := make([]zap.Field, 0, 8)

	loggedFields = append(loggedFields,
		zap.Time("ts_message_received", m.ts),
		zap.Int("message_type", m.msgType),
		zap.Int("message_size", len(m.bytes)),
	)

	if p.cfg.proxy.LogMessages {
		fb := flashblock.Flashblock{}
		err := json.Unmarshal(m.bytes, &fb)

		if len(m.bytes) <= p.cfg.proxy.LogMessagesMaxSize {
			if err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_message", fb),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.NamedError("error_unmarshal", err),
					zap.String("websocket_message", utils.Str(m.bytes)),
				)
			}
		}

		transactions := make(triaged.RequestTransactions, 0, len(fb.Diff.Transactions))
		for _, strTx := range fb.Diff.Transactions {
			errs := make([]error, 0)
			if from, tx, err := jrpc.DecodeEthRawTransaction(strTx); err == nil {
				transactions = append(transactions, triaged.RequestTransaction{
					From:  &from,
					To:    tx.To(),
					Hash:  tx.Hash(),
					Nonce: tx.Nonce(),
				})
			} else {
				_, _, err := jrpc.DecodeEthRawTransaction(strTx)
				errs = append(errs, err)
			}

			if len(transactions) > 0 {
				loggedFields = append(loggedFields,
					zap.Array("txs", transactions),
				)
			}
			if len(errs) > 0 {
				loggedFields = append(loggedFields,
					zap.Errors("error_decode", errs),
				)
			}
		}
	}

	loggedFields = append(loggedFields, extraLogFields...)

	return loggedFields
}

func (p *websocketPump) pumpPings(
	from, to *websocket.Conn,
	direction string,
) func(message string) error {
	l := p.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)
	return func(message string) error {
		l.Debug("Passing ping through...",
			zap.String("message", hex.EncodeToString([]byte(message))),
		)
		return to.WriteControl(
			websocket.PingMessage, []byte(message), utils.Deadline(p.cfg.proxy.ControlTimeout),
		)
	}
}

func (p *websocketPump) pumpPongs(
	from, to *websocket.Conn,
	direction string,
) func(message string) error {
	l := p.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)
	return func(message string) error {
		l.Debug("Passing pong through...",
			zap.String("message", hex.EncodeToString([]byte(message))),
		)
		return to.WriteControl(
			websocket.PongMessage, []byte(message), utils.Deadline(p.cfg.proxy.ControlTimeout),
		)
	}
}

func (p *websocketPump) pumpCloseMessages(
	from, to *websocket.Conn,
	direction string,
) func(code int, message string) error {
	l := p.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)
	return func(code int, message string) error {
		l.Info("Passing close message through...",
			zap.Int("code", code),
			zap.String("message", hex.EncodeToString([]byte(message))),
		)
		return to.WriteControl(
			code, []byte(message), utils.Deadline(p.cfg.proxy.ControlTimeout),
		)
	}
}
