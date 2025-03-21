package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type Chaos struct {
	Enabled bool `yaml:"enabled"`

	MinInjectedLatency           time.Duration `yaml:"min_injected_latency"`
	MaxInjectedLatency           time.Duration `yaml:"max_injected_latency"`
	InjectedHttpErrorProbability float64       `yaml:"injected_http_error_probability"`
	InjectedJrpcErrorProbability float64       `yaml:"injected_jrpc_error_probability"`
}

var (
	errChaosMustBeEnabled                       = errors.New("chaos must be explicitly enabled for this setting to be in effect")
	errChaosInvalidMinInjectedLatency           = errors.New("invalid min injected latency")
	errChaosInvalidMaxInjectedLatency           = errors.New("invalid max injected latency")
	errChaosInvalidInjectedHttpErrorProbability = errors.New("invalid injected http error probability (must be in [0, 100] range)")
	errChaosInvalidInjectedJrpcErrorProbability = errors.New("invalid injected jrpc error probability (must be in [0, 100] range)")
)

func (cfg *Chaos) Validate() error {
	errs := make([]error, 0)

	{ // enabled
		if !cfg.Enabled {
			if cfg.MinInjectedLatency != 0 {
				errs = append(errs, fmt.Errorf("%w: min injected latency",
					errChaosMustBeEnabled,
				))
			}
			if cfg.MaxInjectedLatency != 0 {
				errs = append(errs, fmt.Errorf("%w: max injected latency",
					errChaosMustBeEnabled,
				))
			}
			if cfg.InjectedHttpErrorProbability != 0 {
				errs = append(errs, fmt.Errorf("%w: injected http error probability",
					errChaosMustBeEnabled,
				))
			}
			if cfg.InjectedJrpcErrorProbability != 0 {
				errs = append(errs, fmt.Errorf("%w: injected jrpc error probability",
					errChaosMustBeEnabled,
				))
			}
		}
	}

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

	return utils.FlattenErrors(errs)
}
