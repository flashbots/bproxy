package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type Chaos struct {
	Enabled bool `yaml:"enabled"`

	MinInjectedLatency                     time.Duration `yaml:"min_injected_latency"`
	MaxInjectedLatency                     time.Duration `yaml:"max_injected_latency"`
	InjectedHttpErrorProbability           float64       `yaml:"injected_http_error_probability"`
	InjectedJrpcErrorProbability           float64       `yaml:"injected_jrpc_error_probability"`
	InjectedInvalidJrpcResponseProbability float64       `yaml:"injected_invalid_jrpc_response_probability"`
}

var (
	errChaosInvalidMinInjectedLatency                     = errors.New("invalid min injected latency")
	errChaosInvalidMaxInjectedLatency                     = errors.New("invalid max injected latency")
	errChaosInvalidInjectedHttpErrorProbability           = errors.New("injected http error probability must be in [0, 100] range")
	errChaosInvalidInjectedJrpcErrorProbability           = errors.New("injected jrpc error probability must be in [0, 100] range")
	errChaosInvalidInjectedInvalidJrpcResponseProbability = errors.New("injected invalid jrpc error probability must be in [0, 100] range")
)

func (cfg *Chaos) Validate() error {
	errs := make([]error, 0)

	{ // min injected latency
		if cfg.MinInjectedLatency < 0 {
			errs = append(errs, fmt.Errorf("%w: can not be negative: %s",
				errChaosInvalidMinInjectedLatency,
				cfg.MaxInjectedLatency.String(),
			))
		}
		if cfg.MinInjectedLatency > time.Minute {
			errs = append(errs, fmt.Errorf("%w: can not be more than 1 minute: %s",
				errChaosInvalidMinInjectedLatency,
				cfg.MaxInjectedLatency.String(),
			))
		}

		if cfg.MaxInjectedLatency == 0 {
			cfg.MaxInjectedLatency = cfg.MinInjectedLatency
		}
	}

	{ // max injected latency
		if cfg.MaxInjectedLatency < 0 {
			errs = append(errs, fmt.Errorf("%w: can not be negative: %s",
				errChaosInvalidMaxInjectedLatency,
				cfg.MaxInjectedLatency.String(),
			))
		}
		if cfg.MaxInjectedLatency > time.Minute {
			errs = append(errs, fmt.Errorf("%w: can not be more than 1 minute: %s",
				errChaosInvalidMaxInjectedLatency,
				cfg.MaxInjectedLatency.String(),
			))
		}
	}

	{ // injected http error probability
		if cfg.InjectedHttpErrorProbability < 0 || cfg.InjectedHttpErrorProbability > 100 {
			errs = append(errs, fmt.Errorf("%w: %f",
				errChaosInvalidInjectedHttpErrorProbability,
				cfg.InjectedHttpErrorProbability,
			))
		}
	}

	{ // injected jrpc error probability
		if cfg.InjectedJrpcErrorProbability < 0 || cfg.InjectedJrpcErrorProbability > 100 {
			errs = append(errs, fmt.Errorf("%w: %f",
				errChaosInvalidInjectedJrpcErrorProbability,
				cfg.InjectedJrpcErrorProbability,
			))
		}
	}

	{ // injected invalid jrpc response probability
		if cfg.InjectedInvalidJrpcResponseProbability < 0 || cfg.InjectedInvalidJrpcResponseProbability > 100 {
			errs = append(errs, fmt.Errorf("%w: %f",
				errChaosInvalidInjectedInvalidJrpcResponseProbability,
				cfg.InjectedInvalidJrpcResponseProbability,
			))
		}
	}
	return utils.FlattenErrors(errs)
}
