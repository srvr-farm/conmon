package config

import (
	"github.com/mcallan/conmon/internal/config"
)

// Duration is a convenience alias for the shared duration wrapper used in sysmon
// configuration.
type Duration = config.Duration
type Config struct {
	Push     PushConfig     `yaml:"push"`
	Identity IdentityConfig `yaml:"identity"`
	System   SystemConfig   `yaml:"system"`
	Services []Service      `yaml:"services"`
}

type PushConfig struct {
	URL      string          `yaml:"url"`
	Job      string          `yaml:"job"`
	Interval config.Duration `yaml:"interval"`
	Timeout  config.Duration `yaml:"timeout"`
}

type IdentityConfig struct {
	Host string `yaml:"host"`
}

type SystemConfig struct {
	CollectPerCoreCPU bool `yaml:"collect_per_core_cpu"`
}

type Service struct {
	Name string `yaml:"name"`
}

func (c *Config) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for _, svc := range c.Services {
		names = append(names, svc.Name)
	}
	return names
}
