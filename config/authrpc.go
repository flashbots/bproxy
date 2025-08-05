package config

type Authrpc struct {
	*HttpProxy `yaml:"proxy"`

	DeduplicateFCUs  bool `yaml:"deduplicate_fcus"`
	MirrorGetPayload bool `yaml:"mirror_get_payload"`
}
