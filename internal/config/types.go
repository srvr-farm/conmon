package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a string")
	}
	if value.Tag == "!!null" || strings.TrimSpace(value.Value) == "" {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	if parsed < 0 {
		return fmt.Errorf("duration must be non-negative")
	}
	d.Duration = parsed
	return nil
}

type Config struct {
	Defaults Defaults `yaml:"defaults"`
	Groups   []Group  `yaml:"groups"`
	Export   Export   `yaml:"export"`
}

type Defaults struct {
	Interval Duration          `yaml:"interval"`
	Timeout  Duration          `yaml:"timeout"`
	DNS      DNSDefaults       `yaml:"dns"`
	TLS      TLSDefaults       `yaml:"tls"`
	Labels   map[string]string `yaml:"labels"`
}

type DNSDefaults struct {
	Server string `yaml:"server"`
}

type TLSDefaults struct {
	MinDaysRemaining int `yaml:"min_days_remaining"`
}

type Export struct {
	ListenAddress string `yaml:"listen_address"`
}

type Group struct {
	Name   string  `yaml:"name"`
	Checks []Check `yaml:"checks"`
}

type Check struct {
	ID                 string            `yaml:"id"`
	Name               string            `yaml:"name"`
	Kind               string            `yaml:"kind"`
	Scope              string            `yaml:"scope"`
	Group              string            `yaml:"-"`
	Host               string            `yaml:"host,omitempty"`
	Port               int               `yaml:"port,omitempty"`
	URL                string            `yaml:"url,omitempty"`
	Method             string            `yaml:"method,omitempty"`
	ExpectedStatus     []int             `yaml:"expected_status,omitempty"`
	Headers            map[string]string `yaml:"headers,omitempty"`
	Server             string            `yaml:"server,omitempty"`
	QueryName          string            `yaml:"query_name,omitempty"`
	RecordType         string            `yaml:"record_type,omitempty"`
	ExpectedRCode      string            `yaml:"expected_rcode,omitempty"`
	ExpectedRCodeValue int               `yaml:"-"`
	TLSServerName      string            `yaml:"tls_server_name,omitempty"`
	ServerName         string            `yaml:"server_name,omitempty"`
	MinDaysRemaining   int               `yaml:"min_days_remaining,omitempty"`
	Count              int               `yaml:"count,omitempty"`
	Interval           Duration          `yaml:"interval,omitempty"`
	Timeout            Duration          `yaml:"timeout,omitempty"`
	Labels             map[string]string `yaml:"labels,omitempty"`
}
