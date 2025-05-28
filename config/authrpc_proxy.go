package config

type AuthrpcProxy struct {
	*HttpProxy `yaml:"proxy"`

	DeduplicateFCUs bool `yaml:"deduplicate_fcus"`
}
