package config

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type Proxy struct {
	BackendTimeout                  time.Duration `yaml:"backend_timeout"`
	BackendURL                      string        `yaml:"backend_url"`
	ClientIdleConnectionTimeout     time.Duration `yaml:"client_idle_connection_timeout"`
	Enabled                         bool          `yaml:"enabled"`
	ExtraMirroredJrpcMethods        []string      `yaml:"extra_mirrored_jrpc_methods"`
	HealthcheckInterval             time.Duration `yaml:"healthcheck_interval"`
	HealthcheckThresholdHealthy     int           `yaml:"healthcheck_threshold_healthy"`
	HealthcheckThresholdUnhealthy   int           `yaml:"healthcheck_threshold_unhealthy"`
	HealthcheckURL                  string        `yaml:"healthcheck_url"`
	ListenAddress                   string        `yaml:"listen_address"`
	LogRequests                     bool          `yaml:"log_requests"`
	LogResponses                    bool          `yaml:"log_responses"`
	MaxBackendConnectionsPerHost    int           `yaml:"max_backend_connections_per_host"`
	MaxBackendConnectionWaitTimeout time.Duration `yaml:"max_client_connection_wait_timeout"`
	MaxClientConnectionsPerIP       int           `yaml:"max_client_connections_per_ip"`
	MaxRequestSizeMb                int           `yaml:"max_request_size_mb"`
	MaxResponseSizeMb               int           `yaml:"max_request_size_mb"`
	PeerTLSInsecureSkipVerify       bool          `yaml:"peer_tls_insecure_skip_verify"`
	PeerURLs                        []string      `yaml:"peer_urls"`
	RemoveBackendFromPeers          bool          `yaml:"remove_backend_from_peers"`
	TLSCertificate                  string        `yaml:"tls_crt"`
	TLSKey                          string        `yaml:"tls_key"`
	UsePriorityQueue                bool          `yaml:"use_priority_queue"`
}

var (
	errProxyFailedToGetLocalIPs                    = errors.New("failed to get local ip addresses")
	errProxyInvalidBackendTimeout                  = errors.New("invalid backend timeout")
	errProxyInvalidBackendURL                      = errors.New("invalid backend url")
	errProxyInvalidClientIdleConnectionTimeout     = errors.New("invalid client connection idle timeout")
	errProxyInvalidHealthcheckInterval             = errors.New("invalid healthcheck interval")
	errProxyInvalidHealthcheckThresholdHealthy     = errors.New("invalid healthcheck healthy threshold")
	errProxyInvalidHealthcheckThresholdUnhealthy   = errors.New("invalid healthcheck unhealthy threshold")
	errProxyInvalidHealthcheckURL                  = errors.New("invalid healthcheck url")
	errProxyInvalidListenAddress                   = errors.New("invalid proxy listen address")
	errProxyInvalidMaxBackendConnectionsPerHost    = errors.New("invalid max backend connections per host")
	errProxyInvalidMaxBackendConnectionWaitTimeout = errors.New("invalid max backend connection wait timeout")
	errProxyInvalidMaxClientConnectionsPerIP       = errors.New("invalid max client connections per ip")
	errProxyInvalidMaxRequestSize                  = errors.New("invalid max request size")
	errProxyInvalidMaxResponseSize                 = errors.New("invalid max response size")
	errProxyInvalidPeerURL                         = errors.New("invalid peer url")
	errProxyInvalidTLSConfig                       = errors.New("invalid tls configuration")
)

func (cfg *Proxy) Validate() error {
	errs := make([]error, 0)

	{ // BackendTimeout
		if cfg.BackendTimeout <= 0 {
			errs = append(errs, fmt.Errorf("%w: can't be negative: %s",
				errProxyInvalidBackendTimeout, cfg.BackendTimeout,
			))
		}
		if cfg.BackendTimeout > 30*time.Second {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=30s: %s",
				errProxyInvalidBackendTimeout, cfg.BackendTimeout,
			))
		}
	}

	{ // BackendURL + PeerURLs
		var localIPs []net.IP
		if cfg.RemoveBackendFromPeers {
			var err error
			localIPs, err = utils.LocalIPs()
			if err != nil {
				errs = append(errs, fmt.Errorf("%w: %s: %w",
					errProxyFailedToGetLocalIPs, cfg.ListenAddress, err,
				))
			}
		}

		backendURL, err := url.Parse(cfg.BackendURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w:",
				errProxyInvalidBackendURL, cfg.BackendURL, err,
			))
		}

		backendIPs, err := net.LookupIP(backendURL.Hostname())
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w:",
				errProxyInvalidBackendURL, cfg.BackendURL, err,
			))
		}

		idx := 0
		for _, p := range cfg.PeerURLs {
			peerURL, err := url.Parse(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("%w: %s: %w",
					errProxyInvalidPeerURL, p, err,
				))
				continue
			}
			peerIPs, err := net.LookupIP(peerURL.Hostname())
			if err != nil {
				continue
			}

			if cfg.RemoveBackendFromPeers && backendURL != nil {
				if peerURL.Host == backendURL.Host {
					continue // if backend and peer hostnames are exact match, drop the peer
				}

				if peerURL.Port() == backendURL.Port() && utils.IPsMatch(peerIPs, backendIPs) {
					continue // if peer resolves to same addresses as backend does, drop the peer
				}

				// if ports don't match, keep the peer
				if peerURL.Port() == backendURL.Port() && utils.IsLoopback(backendIPs) && utils.IsLoopback(peerIPs) {
					continue // if backend and peer are both loopbacks, drop the peer
				}

				if peerURL.Port() == backendURL.Port() && utils.IsLoopback(backendIPs) && utils.IPsOverlap(peerIPs, localIPs) {
					continue // if backend is loopback and peer resolves to one of the local addresses, drop the peer
				}
			}

			cfg.PeerURLs[idx] = p
			idx++
		}
		cfg.PeerURLs = cfg.PeerURLs[:idx]
	}

	{ // ClientIdleConnectionTimeout
		if cfg.ClientIdleConnectionTimeout <= 0 {
			errs = append(errs, fmt.Errorf("%w: can't be negative: %s",
				errProxyInvalidClientIdleConnectionTimeout, cfg.ClientIdleConnectionTimeout,
			))
		}
		if cfg.ClientIdleConnectionTimeout > 60*time.Hour {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=60m: %s",
				errProxyInvalidClientIdleConnectionTimeout, cfg.ClientIdleConnectionTimeout,
			))
		}
	}

	{ // HealthcheckInterval
		if cfg.HealthcheckInterval < time.Second {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1s: %s",
				errProxyInvalidHealthcheckInterval, cfg.HealthcheckInterval,
			))
		}
		if cfg.HealthcheckInterval > time.Minute {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=1m: %s",
				errProxyInvalidHealthcheckInterval, cfg.HealthcheckInterval,
			))
		}
	}

	{ // HealthcheckThresholdHealthy
		if cfg.HealthcheckThresholdHealthy < 1 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1: %d",
				errProxyInvalidHealthcheckThresholdHealthy, cfg.HealthcheckThresholdHealthy,
			))
		}
		if cfg.HealthcheckThresholdHealthy > 10 {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=10: %d",
				errProxyInvalidHealthcheckThresholdHealthy, cfg.HealthcheckThresholdHealthy,
			))
		}
	}

	{ // HealthcheckThresholdUnhealthy
		if cfg.HealthcheckThresholdUnhealthy < 1 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1: %d",
				errProxyInvalidHealthcheckThresholdUnhealthy, cfg.HealthcheckThresholdUnhealthy,
			))
		}
		if cfg.HealthcheckThresholdUnhealthy > 10 {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=10: %d",
				errProxyInvalidHealthcheckThresholdUnhealthy, cfg.HealthcheckThresholdUnhealthy,
			))
		}
	}

	{ // HealthcheckURL
		if cfg.HealthcheckURL != "" {
			if _, err := url.Parse(cfg.HealthcheckURL); err != nil {
				errs = append(errs, fmt.Errorf("%w: %w",
					errProxyInvalidHealthcheckURL, err,
				))
			}
		}
	}

	{ // ListenAddress
		if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w",
				errProxyInvalidListenAddress, cfg.ListenAddress, err,
			))
		}
	}

	{ // MaxBackendConnectionsPerHost
		if cfg.MaxBackendConnectionsPerHost < 0 {
			errs = append(errs, fmt.Errorf("%w: can't be negative: %d",
				errProxyInvalidMaxBackendConnectionsPerHost, cfg.MaxBackendConnectionsPerHost,
			))
		}
		if cfg.MaxBackendConnectionsPerHost > 1024 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=1024: %d",
				errProxyInvalidMaxBackendConnectionsPerHost, cfg.MaxBackendConnectionsPerHost,
			))
		}
	}

	{ // MaxBackendConnectionWaitTimeout
		if cfg.MaxBackendConnectionWaitTimeout < 0 {
			errs = append(errs, fmt.Errorf("%w: can't be negative: %s",
				errProxyInvalidMaxBackendConnectionWaitTimeout, cfg.MaxBackendConnectionWaitTimeout,
			))
		}
		if cfg.MaxBackendConnectionWaitTimeout > time.Minute {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=1m: %s",
				errProxyInvalidMaxBackendConnectionWaitTimeout, cfg.MaxBackendConnectionWaitTimeout,
			))
		}
	}

	{ // MaxClientConnectionsPerIP
		if cfg.MaxClientConnectionsPerIP < 0 {
			errs = append(errs, fmt.Errorf("%w: can't be negative: %d",
				errProxyInvalidMaxClientConnectionsPerIP, cfg.MaxClientConnectionsPerIP,
			))
		}
		if cfg.MaxClientConnectionsPerIP > 1024 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=1024: %d",
				errProxyInvalidMaxClientConnectionsPerIP, cfg.MaxClientConnectionsPerIP,
			))
		}
	}

	{ // MaxRequestSizeMb
		if cfg.MaxRequestSizeMb < 4 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=4: %d",
				errProxyInvalidMaxRequestSize, cfg.MaxRequestSizeMb,
			))
		}
		if cfg.MaxRequestSizeMb > 4096 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=4096: %d",
				errProxyInvalidMaxRequestSize, cfg.MaxRequestSizeMb,
			))
		}
	}

	{ // MaxResponseSizeMb
		if cfg.MaxResponseSizeMb < 4 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=4: %d",
				errProxyInvalidMaxResponseSize, cfg.MaxResponseSizeMb,
			))
		}
		if cfg.MaxResponseSizeMb > 4096 {
			errs = append(errs, fmt.Errorf("%w: too high, must be <=4096: %d",
				errProxyInvalidMaxResponseSize, cfg.MaxResponseSizeMb,
			))
		}
	}

	{ // TLSCertificate + TLSKey
		if cfg.TLSCertificate != "" || cfg.TLSKey != "" {
			if cfg.TLSCertificate == "" {
				errs = append(errs, fmt.Errorf("%w: tls certificate must also be configured",
					errProxyInvalidTLSConfig,
				))
			} else if cfg.TLSKey == "" {
				errs = append(errs, fmt.Errorf("%w: tls key must also be configured",
					errProxyInvalidTLSConfig,
				))
			} else if _, err := cfg.LoadTLSCertificate(); err != nil {
				errs = append(errs, fmt.Errorf("%w: %w",
					errProxyInvalidTLSConfig, err,
				))
			}
		}
	}

	return utils.FlattenErrors(errs)
}

func (cfg *Proxy) LoadTLSCertificate() (tls.Certificate, error) {
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
