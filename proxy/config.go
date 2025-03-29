package proxy

import "github.com/flashbots/bproxy/config"

type Config struct {
	Name string

	Chaos *config.Chaos
	Proxy *config.Proxy
}
