package main

import (
	"flag"

	"github.com/BurntSushi/toml"
	"github.com/asticode/go-astilog"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
)

var (
	configPath = flag.String("c", "", "the config path")
)

type Configuration struct {
	Logger  astilog.Configuration `toml:"logger"`
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
		Logger: astilog.Configuration{
			AppName: "astiencoder",
		},
	}

	// Local
	if *configPath != "" {
		if _, err = toml.DecodeFile(*configPath, &c); err != nil {
			err = errors.Wrapf(err, "main: toml decoding %s failed", *configPath)
			return
		}
	}

	// Flag
	if err = mergo.Merge(&c, Configuration{
		Logger: astilog.FlagConfig(),
	}); err != nil {
		err = errors.Wrap(err, "main: merging flag config failed")
		return
	}
	return
}
