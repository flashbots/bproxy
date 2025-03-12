package logutils

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/flashbots/bproxy/config"
)

var (
	errLoggerFailedToBuild = errors.New("failed to build the logger")
	errLoggerInvalidLevel  = errors.New("invalid log-level")
	errLoggerInvalidMode   = errors.New("invalid log-mode")
)

func NewLogger(cfg *config.Log) (
	*zap.Logger, error,
) {
	var config zap.Config
	switch strings.ToLower(cfg.Mode) {
	case "dev":
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeCaller = nil
	case "prod":
		config = zap.NewProductionConfig()
	default:
		return nil, fmt.Errorf("%w: %s",
			errLoggerInvalidMode, cfg.Mode,
		)
	}
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logLevel, err := zap.ParseAtomicLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w",
			errLoggerInvalidLevel, cfg.Level, err,
		)
	}
	config.Level = logLevel

	l, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("%w: %w",
			errLoggerFailedToBuild, err,
		)
	}

	return l, nil
}
