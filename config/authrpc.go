package config

type Authrpc struct {
	*HttpProxy `yaml:"proxy"`

	DeduplicateFCUs bool `yaml:"deduplicate_fcus"`

	MirrorFcuWithoutPayload bool `yaml:"mirror_fcu_without_payload"`
	MirrorFcuWithPayload    bool `yaml:"mirror_fcu_with_payload"`
	MirrorGetPayload        bool `yaml:"mirror_get_payload"`
	MirrorNewPayload        bool `yaml:"mirror_new_payload"`
	MirrorSetMaxDASize      bool `yaml:"mirror_set_max_da_size"`
}
