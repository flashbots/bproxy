package config

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

type Log struct {
	Level string `yaml:"level"`
	Mode  string `yaml:"mode"`
}

var (
	errLogInvalidLevel = errors.New("invalid log-level")
	errLogInvalidMode  = errors.New("invalid log-mode")
)

func (cfg *Log) Validate() error {
	if cfg.Mode != "dev" && cfg.Mode != "prod" {
		return fmt.Errorf("%w: %s",
			errLogInvalidMode, cfg.Mode,
		)
	}

	if _, err := zap.ParseAtomicLevel(cfg.Level); err != nil {
		return fmt.Errorf("%w: %s: %w",
			errLogInvalidLevel, cfg.Level, err,
		)
	}

	return nil
}
