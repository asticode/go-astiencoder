package main

import (
	"flag"
	"fmt"

	"github.com/BurntSushi/toml"
)

var (
	configPath = flag.String("c", "", "the config path")
)

type Configuration struct {
	Encoder *ConfigurationEncoder `toml:"encoder"`
}

type ConfigurationEncoder struct {
	Exec   ConfigurationExec   `toml:"exec"`
	Server ConfigurationServer `toml:"server"`
}

type ConfigurationExec struct {
	StopWhenWorkflowsAreStopped bool `toml:"stop_when_workflows_are_stopped"`
}

type ConfigurationServer struct {
	Addr    string `toml:"addr"`
	PathWeb string `toml:"path_web"`
}

func newConfiguration() (c Configuration, err error) {
	// Global
	c = Configuration{
		Encoder: &ConfigurationEncoder{
			Server: ConfigurationServer{
				Addr:    "127.0.0.1:4000",
				PathWeb: "web",
			},
		},
	}

	// Local
	if *configPath != "" {
		if _, err = toml.DecodeFile(*configPath, &c); err != nil {
			err = fmt.Errorf("main: toml decoding %s failed: %w", *configPath, err)
			return
		}
	}
	return
}
