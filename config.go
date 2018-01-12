package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/kelseyhightower/envconfig"

	log "github.com/Sirupsen/logrus"
)

// Config holds the configuration passed in via env vars.
type Config struct {
	MesosHost       ConfigHost    `required:"true"`
	MarathonHost    ConfigHost    `required:"true"`
	ServerPort      int           `default:"8889"`
	UpdateFrequency time.Duration `default:"5m0s"`
	DryRun          bool          `default:"false"`
	Debug           bool          `default:"false"`
	SkipTls         bool          `default:"true"`
	Authorization   string        `default:""`
}

// ConfigHost represents a host String.
type ConfigHost string

// Decode allows envconfig to validate the provided host string.
func (ch *ConfigHost) Decode(value string) error {
	uri, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("Invalid URL (%s): %s", value, err)
	}

	if uri.Host == "" {
		return fmt.Errorf("Empty host in URL")
	}

	*ch = ConfigHost(value)

	return nil
}

func init() {
	err := envconfig.Process("daemonset", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}
	log.Debugf("%+v", config)
}
