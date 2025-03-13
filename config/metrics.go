package config

import (
	"errors"
	"fmt"
	"net"
)

var (
	errMetricsInvalidListenAddress = errors.New("invalid metrics listen address")
)

type Metrics struct {
	ListenAddress string `yaml:"listen_address"`
}

func (cfg *Metrics) Validate() error {
	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
		return fmt.Errorf("%w: %s: %w",
			errMetricsInvalidListenAddress, cfg.ListenAddress, err,
		)
	}

	return nil
}
