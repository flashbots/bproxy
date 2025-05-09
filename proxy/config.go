package proxy

import "github.com/flashbots/bproxy/config"

type proxyConfig struct {
	name string

	chaos *config.Chaos
	proxy *config.Proxy
}

type authrpcProxyConfig struct {
	deduplicateFCUs bool
}
