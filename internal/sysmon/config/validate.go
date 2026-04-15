package config

import (
	"fmt"
	"net/url"
	"strings"
)

func (c *Config) Validate() error {
	c.Push.URL = strings.TrimSpace(c.Push.URL)
	c.Push.Job = strings.TrimSpace(c.Push.Job)
	if c.Push.Job == "" {
		c.Push.Job = "sysmon"
	}
	if c.Push.URL == "" {
		return fmt.Errorf("push.url is required")
	}
	parsed, err := url.Parse(c.Push.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("push.url must be absolute with scheme and host")
	}
	if strings.ToLower(parsed.Scheme) != "http" {
		return fmt.Errorf("push.url scheme must be http")
	}
	if c.Push.Interval.Duration <= 0 {
		return fmt.Errorf("push.interval must be greater than zero")
	}
	if c.Push.Timeout.Duration <= 0 {
		return fmt.Errorf("push.timeout must be greater than zero")
	}

	c.Identity.Host = strings.TrimSpace(c.Identity.Host)

	if c.System.CollectPerCoreCPU == nil {
		defaultFlag := true
		c.System.CollectPerCoreCPU = &defaultFlag
	}

	seen := make(map[string]int, len(c.Services))
	for i := range c.Services {
		svc := &c.Services[i]
		svc.Name = strings.TrimSpace(svc.Name)
		if svc.Name == "" {
			return fmt.Errorf("services[%d] name is required", i)
		}
		if firstIndex, exists := seen[svc.Name]; exists {
			return fmt.Errorf("services[%d] name %q is duplicated (first at %d)", i, svc.Name, firstIndex)
		}
		seen[svc.Name] = i
	}

	return nil
}
