package config

// DefaultConfig returns the in-memory default Config value.
//
// Runtime configuration is not built from this: the live cascade is
// viper, seeded by setConfigDefaults in internal/cli. This constructor
// exists for tests and for callers needing a fully-populated Config
// without touching the filesystem. Adding a field here does NOT make it
// readable at runtime — register a viper default instead.
func DefaultConfig() *Config {
	return &Config{
		Version: "0.1",
		Output: OutputConfig{
			Format:  "table",
			Color:   true,
			Verbose: false,
		},
		Task: TaskConfig{
			DefaultStatus: "TODO",
			Statuses:      GetDefaultStatuses(),
			StateMachine:  GetDefaultStateMachine(),
		},
		Git: GitConfig{
			Track: false,
		},
		Storage: StorageConfig{
			Backend: "sqlite",
			DBPath:  ".tlc/db.sqlite",
		},
		UI: UIConfig{
			Timezone:   "local",
			TableStyle: "unicode",
		},
	}
}
