package config

type Authrpc struct {
	*HttpProxy `yaml:"proxy"`

	DeduplicateFCUs bool `yaml:"deduplicate_fcus"`
}
