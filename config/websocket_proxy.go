package config

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/flashbots/bproxy/utils"
)

type WebsocketProxy struct {
	BackendURL         string       `yaml:"backend_url"`
	Enabled            bool         `yaml:"enabled"`
	Healthcheck        *Healthcheck `yaml:"healthcheck"`
	ListenAddress      string       `yaml:"listen_address"`
	LogMessages        bool         `yaml:"log_messages"`
	LogMessagesMaxSize int          `yaml:"log_messages_max_size"`
	ReadBufferSize     int          `yaml:"read_buffer_size_mb"`
	TLSCertificate     string       `yaml:"tls_crt"`
	TLSKey             string       `yaml:"tls_key"`
	WriteBufferSize    int          `yaml:"write_buffer_size_mb"`
}

var (
	errWebsocketProxyInvalidBackendURL      = errors.New("invalid backend url")
	errWebsocketProxyInvalidListenAddress   = errors.New("invalid proxy listen address")
	errWebsocketProxyInvalidReadBufferSize  = errors.New("invalid read buffer size")
	errWebsocketProxyInvalidTLSConfig       = errors.New("invalid tls configuration")
	errWebsocketProxyInvalidWriteBufferSize = errors.New("invalid read buffer size")
)

func (cfg *WebsocketProxy) Validate() error {
	errs := make([]error, 0)

	{ // BackendURL
		_, err := url.Parse(cfg.BackendURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w:",
				errWebsocketProxyInvalidBackendURL, cfg.BackendURL, err,
			))
		}
	}

	{ // ListenAddress
		if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w",
				errWebsocketProxyInvalidListenAddress, cfg.ListenAddress, err,
			))
		}
	}

	{ // ReadBufferSize
		if cfg.ReadBufferSize < 4 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=4: %d",
				errWebsocketProxyInvalidReadBufferSize, cfg.ReadBufferSize,
			))
		}
		if cfg.ReadBufferSize > 4096 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=4096: %d",
				errWebsocketProxyInvalidReadBufferSize, cfg.ReadBufferSize,
			))
		}
	}

	{ // TLSCertificate + TLSKey
		if cfg.TLSCertificate != "" || cfg.TLSKey != "" {
			if cfg.TLSCertificate == "" {
				errs = append(errs, fmt.Errorf("%w: tls certificate must also be configured",
					errWebsocketProxyInvalidTLSConfig,
				))
			} else if cfg.TLSKey == "" {
				errs = append(errs, fmt.Errorf("%w: tls key must also be configured",
					errWebsocketProxyInvalidTLSConfig,
				))
			} else if _, err := cfg.LoadTLSCertificate(); err != nil {
				errs = append(errs, fmt.Errorf("%w: %w",
					errWebsocketProxyInvalidTLSConfig, err,
				))
			}
		}
	}

	{ // WriteBufferSize
		if cfg.WriteBufferSize < 4 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=4: %d",
				errWebsocketProxyInvalidWriteBufferSize, cfg.WriteBufferSize,
			))
		}
		if cfg.WriteBufferSize > 4096 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=4096: %d",
				errWebsocketProxyInvalidWriteBufferSize, cfg.WriteBufferSize,
			))
		}
	}

	return utils.FlattenErrors(errs)
}

func (cfg *WebsocketProxy) LoadTLSCertificate() (tls.Certificate, error) {
	crt, err := os.ReadFile(cfg.TLSCertificate)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := os.ReadFile(cfg.TLSKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	if debase64, err := base64.StdEncoding.DecodeString(string(crt)); err == nil {
		crt = debase64
	}
	if debase64, err := base64.StdEncoding.DecodeString(string(key)); err == nil {
		key = debase64
	}

	return tls.X509KeyPair(crt, key)
}
