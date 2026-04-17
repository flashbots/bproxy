package config

import (
	"errors"
	"fmt"
	"net"
)

var (
	errPprofInvalidListenAddress = errors.New("invalid pprof listen address")
)

type Pprof struct {
	ListenAddress string `yaml:"listen_address"`
}

func (cfg *Pprof) Pprof() error {
	if cfg.ListenAddress != "" {
		if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
			return fmt.Errorf("%w: %s: %w",
				errPprofInvalidListenAddress, cfg.ListenAddress, err,
			)
		}
	}

	return nil
}
