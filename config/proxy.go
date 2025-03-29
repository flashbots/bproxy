package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type Proxy struct {
	Backend                         string        `yaml:"backend"`
	Enabled                         bool          `yaml:"enabled"`
	ListenAddress                   string        `yaml:"listen_address"`
	LogRequests                     bool          `yaml:"log_requests"`
	LogResponses                    bool          `yaml:"log_responses"`
	MaxBackendConnectionsPerHost    int           `yaml:"max_backend_connections_per_host"`
	MaxBackendConnectionWaitTimeout time.Duration `yaml:"max_client_connection_wait_timeout"`
	MaxClientConnectionsPerIP       int           `yaml:"max_client_connections_per_ip"`
	MaxRequestSize                  int           `yaml:"max_request_size"`
	MaxResponseSize                 int           `yaml:"max_request_size"`
	Peers                           []string      `yaml:"peers"`
	RemoveBackendFromPeers          bool          `yaml:"remove_backend_from_peers"`
}

var (
	errProxyFailedToGetLocalIPs  = errors.New("failed to get local ip addresses")
	errProxyInvalidBackend       = errors.New("invalid backend url")
	errProxyInvalidListenAddress = errors.New("invalid proxy listen address")
	errProxyInvalidPeer          = errors.New("invalid peer url")
)

func (cfg *Proxy) Validate() error {
	errs := make([]error, 0)

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

	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
		errs = append(errs, fmt.Errorf("%w: %s: %w",
			errProxyInvalidListenAddress, cfg.ListenAddress, err,
		))
	}

	backend, err := url.Parse(cfg.Backend)
	if err != nil {
		errs = append(errs, fmt.Errorf("%w: %s: %w:",
			errProxyInvalidBackend, cfg.Backend, err,
		))
	}
	backendIPs, err := net.LookupIP(backend.Hostname())
	if err != nil {
		errs = append(errs, fmt.Errorf("%w: %s: %w:",
			errProxyInvalidBackend, cfg.Backend, err,
		))
	}

	idx := 0
	for _, p := range cfg.Peers {
		peer, err := url.Parse(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w",
				errProxyInvalidPeer, p, err,
			))
			continue
		}
		peerIPs, err := net.LookupIP(peer.Hostname())
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %s: %w",
				errProxyInvalidPeer, p, err,
			))
			continue
		}

		if cfg.RemoveBackendFromPeers && backend != nil {
			if peer.Host == backend.Host {
				continue // if backend and peer hostnames are exact match, drop the peer
			}

			if peer.Port() == backend.Port() && utils.IPsMatch(peerIPs, backendIPs) {
				continue // if peer resolves to same addresses as backend does, drop the peer
			}

			// if ports don't match, keep the peer
			if peer.Port() == backend.Port() && utils.IsLoopback(backendIPs) && utils.IsLoopback(peerIPs) {
				continue // if backend and peer are both loopbacks, drop the peer
			}

			if peer.Port() == backend.Port() && utils.IsLoopback(backendIPs) && utils.IPsOverlap(peerIPs, localIPs) {
				continue // if backend is loopback and peer resolves to one of the local addresses, drop the peer
			}
		}

		cfg.Peers[idx] = p
		idx++
	}
	cfg.Peers = cfg.Peers[:idx]

	switch len(errs) {
	default:
		return errors.Join(errs...)
	case 1:
		return errs[0]
	case 0:
		return nil
	}
}
