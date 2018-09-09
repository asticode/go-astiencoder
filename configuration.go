package astiencoder

// Configuration represents an encoder configuration
type Configuration struct {
	Exec   ConfigurationExec   `toml:"exec"`
	Server ConfigurationServer `toml:"server"`
}

// ConfigurationExec represents an exec configuration
type ConfigurationExec struct {
	StopWhenWorkflowsAreDone bool `toml:"stop_when_workflows_are_done"`
}

// ConfigurationServer represents a server configuration
type ConfigurationServer struct {
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	PathWeb  string `toml:"path_web"`
	Username string `toml:"username"`
}

// FlagConfig represents flag configuration
func FlagConfig() *Configuration {
	return &Configuration{}
}
