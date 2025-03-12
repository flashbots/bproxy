package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

type Proxy struct {
	BackendAuthRPC string `yaml:"backend_authrpc"`
	BackendRPC     string `yaml:"backend_rpc"`

	ListenAddressAuthRPC string `yaml:"listen_address_authrpc"`
	ListenAddressRPC     string `yaml:"listen_address_rpc"`

	PeersAuthRPC []string `yaml:"peers_authrpc"`
	PeersRPC     []string `yaml:"peers_rpc"`
}

var (
	errServerInvalidBackendAuthRPC = errors.New("invalid url for backend authrpc")
	errServerInvalidBackendRPC     = errors.New("invalid url for backend rpc")

	errServerInvalidListenAddressAuthPC = errors.New("invalid authrpc listen address")
	errServerInvalidListenAddressRPC    = errors.New("invalid rpc listen address")

	errServerInvalidPeerAuthRPC = errors.New("invalid url for peer authrpc")
	errServerInvalidPeerRPC     = errors.New("invalid url for peer rpc")
)

func (cfg *Proxy) Validate() error {
	errs := make([]error, 0)

	if _, err := url.Parse(cfg.BackendAuthRPC); err != nil {
		errs = append(errs, fmt.Errorf("%w: %w",
			errServerInvalidBackendAuthRPC, err,
		))
	}

	if _, err := url.Parse(cfg.BackendRPC); err != nil {
		errs = append(errs, fmt.Errorf("%w: %w",
			errServerInvalidBackendRPC, err,
		))
	}
	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddressAuthRPC); err != nil {
		errs = append(errs, fmt.Errorf("%w: %w",
			errServerInvalidListenAddressAuthPC, err,
		))
	}

	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddressRPC); err != nil {
		errs = append(errs, fmt.Errorf("%w: %w",
			errServerInvalidListenAddressRPC, err,
		))
	}

	for _, u := range cfg.PeersAuthRPC {
		if _, err := url.Parse(u); err != nil {
			errs = append(errs, fmt.Errorf("%w: %w: %s",
				errServerInvalidPeerAuthRPC, err, u,
			))
		}
	}

	for _, u := range cfg.PeersRPC {
		if _, err := url.Parse(u); err != nil {
			errs = append(errs, fmt.Errorf("%w: %w: %s",
				errServerInvalidPeerRPC, err, u,
			))
		}
	}

	switch len(errs) {
	default:
		return errors.Join(errs...)
	case 1:
		return errs[0]
	case 0:
		return nil
	}
}
