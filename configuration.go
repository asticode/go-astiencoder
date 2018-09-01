package astiencoder

// Configuration represents an encoder configuration
type Configuration struct {
	Server ConfigurationServer `toml:"server"`
}

// ConfigurationServer represents a server configuration
type ConfigurationServer struct {
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	PathWeb  string `toml:"path_web"`
	Username string `toml:"username"`
}

// FlagConfig represents flag configuration
func FlagConfig() Configuration {
	return Configuration{}
}
