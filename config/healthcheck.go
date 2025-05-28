package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/flashbots/bproxy/utils"
)

type Healthcheck struct {
	Interval           time.Duration `yaml:"interval"`
	ThresholdHealthy   int           `yaml:"threshold_healthy"`
	ThresholdUnhealthy int           `yaml:"threshold_unhealthy"`
	URL                string        `yaml:"url"`
}

var (
	errHealthcheckInvalidInterval           = errors.New("invalid healthcheck interval")
	errHealthcheckInvalidThresholdHealthy   = errors.New("invalid healthcheck healthy threshold")
	errHealthcheckInvalidThresholdUnhealthy = errors.New("invalid healthcheck unhealthy threshold")
	errHealthcheckInvalidURL                = errors.New("invalid healthcheck url")
)

func (cfg *Healthcheck) Validate() error {
	errs := make([]error, 0)

	{ // Interval
		if cfg.Interval < time.Second {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1s: %s",
				errHealthcheckInvalidInterval, cfg.Interval,
			))
		}
		if cfg.Interval > time.Minute {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=1m: %s",
				errHealthcheckInvalidInterval, cfg.Interval,
			))
		}
	}

	{ // ThresholdHealthy
		if cfg.ThresholdHealthy < 1 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1: %d",
				errHealthcheckInvalidThresholdHealthy, cfg.ThresholdHealthy,
			))
		}
		if cfg.ThresholdHealthy > 10 {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=10: %d",
				errHealthcheckInvalidThresholdHealthy, cfg.ThresholdHealthy,
			))
		}
	}

	{ // ThresholdUnhealthy
		if cfg.ThresholdUnhealthy < 1 {
			errs = append(errs, fmt.Errorf("%w: too low, must be >=1: %d",
				errHealthcheckInvalidThresholdUnhealthy, cfg.ThresholdUnhealthy,
			))
		}
		if cfg.ThresholdUnhealthy > 10 {
			errs = append(errs, fmt.Errorf("%w: too low, must be <=10: %d",
				errHealthcheckInvalidThresholdUnhealthy, cfg.ThresholdUnhealthy,
			))
		}
	}

	{ // URL
		if cfg.URL != "" {
			if _, err := url.Parse(cfg.URL); err != nil {
				errs = append(errs, fmt.Errorf("%w: %w",
					errHealthcheckInvalidURL, err,
				))
			}
		}
	}

	return utils.FlattenErrors(errs)
}
