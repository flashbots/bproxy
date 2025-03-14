package logutils

import (
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type fasthttpLogger struct {
	logger *zap.SugaredLogger
}

func FasthttpLogger(logger *zap.Logger) fasthttp.Logger {
	return &fasthttpLogger{
		logger: logger.Sugar(),
	}
}

func (l *fasthttpLogger) Printf(format string, args ...any) {
	l.logger.Infof(format, args...)
}
