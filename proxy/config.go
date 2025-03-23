package proxy

import "github.com/flashbots/bproxy/config"

type Config struct {
	Name          string
	ListenAddress string

	MaxRequestSize  int
	MaxResponseSize int

	BackendURI string
	PeerURIs   []string

	Chaos        *config.Chaos
	LogRequests  bool
	LogResponses bool
}
