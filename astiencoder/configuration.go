package main

import (
	"flag"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/config"
	"github.com/pkg/errors"
)

// Flags
var (
	configPath = flag.String("c", "", "the config path")
)

// Configuration represents a configuration
type Configuration struct {
	Logger  astilog.Configuration      `toml:"logger"`
	Encoder *astiencoder.Configuration `toml:"encoder"`
}

func newConfiguration() (c Configuration, err error) {
	var i interface{}
	if i, err = asticonfig.New(&Configuration{
		Encoder: &astiencoder.Configuration{
			Server: astiencoder.ConfigurationServer{
				Addr:    "127.0.0.1:4000",
				PathWeb: "web",
			},
		},
		Logger: astilog.Configuration{
			AppName: "astiencoder",
		},
	}, *configPath, &Configuration{
		Encoder: astiencoder.FlagConfig(),
		Logger:  astilog.FlagConfig(),
	}); err != nil {
		err = errors.Wrap(err, "main: asticonfig.New failed")
		return
	}
	c = *(i.(*Configuration))
	return
}
