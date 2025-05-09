package config

import (
	"errors"
	"reflect"
)

type Config struct {
	Chaos *Chaos `yaml:"chaos"`
	Log   *Log   `yaml:"log"`

	AuthrpcProxy *AuthrpcProxy `yaml:"authrpc_proxy"`
	RpcProxy     *RpcProxy     `yaml:"rpc_proxy"`

	Metrics *Metrics `yaml:"metrics"`
}

func New() *Config {
	return &Config{
		Chaos: &Chaos{},
		Log:   &Log{},

		AuthrpcProxy: &AuthrpcProxy{Proxy: &Proxy{}},
		RpcProxy:     &RpcProxy{Proxy: &Proxy{}},

		Metrics: &Metrics{},
	}
}

var (
	errConfigNoEnabledProxy = errors.New("neither authrpc nor rpc proxy are enabled, shutting down...")
)

func (c *Config) Validate() error {
	if !c.AuthrpcProxy.Enabled && !c.RpcProxy.Enabled {
		return errConfigNoEnabledProxy
	}

	return validate(c)
}

func validate(item interface{}) error {
	v := reflect.ValueOf(item)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	errs := []error{}
	for idx := 0; idx < v.NumField(); idx++ {
		field := v.Field(idx)

		if field.Kind() == reflect.Ptr && field.IsNil() {
			continue
		}

		if v, ok := field.Interface().(validatee); ok {
			if err := v.Validate(); err != nil {
				errs = append(errs, err)
			}
		}

		if field.Kind() == reflect.Ptr {
			field = field.Elem()
		}

		switch field.Kind() {
		case reflect.Struct:
			if err := validate(field.Interface()); err != nil {
				errs = append(errs, err)
			}
		case reflect.Slice, reflect.Array:
			for jdx := 0; jdx < field.Len(); jdx++ {
				if err := validate(field.Index(jdx).Interface()); err != nil {
					errs = append(errs, err)
				}
			}
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
