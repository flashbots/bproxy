package config

type AuthrpcProxy struct {
	*Proxy

	DeduplicateFCUs bool `yaml:"deduplicate_fcus"`
}
