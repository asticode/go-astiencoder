package main

import (
	"flag"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/config"
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
	var i interface{}
	if i, err = asticonfig.New(&Configuration{
		Encoder: &ConfigurationEncoder{
			Server: ConfigurationServer{
				Addr:    "127.0.0.1:4000",
				PathWeb: "web",
			},
		},
		Logger: astilog.Configuration{
			AppName: "astiencoder",
		},
	}, *configPath, &Configuration{
		Logger: astilog.FlagConfig(),
	}); err != nil {
		err = errors.Wrap(err, "main: asticonfig.New failed")
		return
	}
	c = *(i.(*Configuration))
	return
}
