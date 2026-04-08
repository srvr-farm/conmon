package config

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var rcodeMap = map[string]int{
	"NOERROR":  0,
	"FORMERR":  1,
	"SERVFAIL": 2,
	"NXDOMAIN": 3,
	"NOTIMP":   4,
	"REFUSED":  5,
}

var dnsRecordTypes = map[string]struct{}{
	"A":     {},
	"AAAA":  {},
	"CNAME": {},
	"MX":    {},
	"NS":    {},
	"PTR":   {},
	"SOA":   {},
	"SRV":   {},
	"TXT":   {},
}

var labelKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func (c *Config) Validate() error {
	c.Defaults.DNS.Server = strings.TrimSpace(c.Defaults.DNS.Server)
	if c.Defaults.Interval.Duration <= 0 {
		return fmt.Errorf("defaults.interval must be greater than zero")
	}
	if c.Defaults.Timeout.Duration <= 0 {
		return fmt.Errorf("defaults.timeout must be greater than zero")
	}
	if c.Defaults.TLS.MinDaysRemaining < 0 {
		return fmt.Errorf("defaults.tls.min_days_remaining must be non-negative")
	}
	labels, err := normalizeLabels("defaults.labels", c.Defaults.Labels)
	if err != nil {
		return err
	}
	c.Defaults.Labels = labels
	c.Export.ListenAddress = strings.TrimSpace(c.Export.ListenAddress)
	if c.Export.ListenAddress == "" {
		return fmt.Errorf("export.listen_address is required")
	}
	_, port, err := net.SplitHostPort(c.Export.ListenAddress)
	if err != nil {
		return fmt.Errorf("export.listen_address must be host:port")
	}
	portValue, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("export.listen_address port must be numeric")
	}
	if err := validatePort(portValue); err != nil {
		return fmt.Errorf("export.listen_address %w", err)
	}

	groupNames := make(map[string]struct{}, len(c.Groups))
	checkIDs := make(map[string]struct{})
	for gi := range c.Groups {
		group := &c.Groups[gi]
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			return fmt.Errorf("group[%d] name is required", gi)
		}
		if _, exists := groupNames[group.Name]; exists {
			return fmt.Errorf("group %q is duplicated", group.Name)
		}
		groupNames[group.Name] = struct{}{}
		for ci := range group.Checks {
			check := &group.Checks[ci]
			check.Group = group.Name
			if err := validateCheck(c, group.Name, ci, check); err != nil {
				return err
			}
			if _, exists := checkIDs[check.ID]; exists {
				return fmt.Errorf("check id %q is duplicated", check.ID)
			}
			checkIDs[check.ID] = struct{}{}
		}
	}
	return nil
}

func validateCheck(cfg *Config, groupName string, index int, check *Check) error {
	check.ID = strings.TrimSpace(check.ID)
	check.Name = strings.TrimSpace(check.Name)
	check.Kind = strings.ToLower(strings.TrimSpace(check.Kind))
	check.Scope = strings.TrimSpace(check.Scope)
	check.Host = strings.TrimSpace(check.Host)
	check.URL = strings.TrimSpace(check.URL)
	check.Method = strings.TrimSpace(check.Method)
	check.Server = strings.TrimSpace(check.Server)
	check.QueryName = strings.TrimSpace(check.QueryName)
	check.RecordType = strings.ToUpper(strings.TrimSpace(check.RecordType))
	check.ExpectedRCode = strings.ToUpper(strings.TrimSpace(check.ExpectedRCode))
	check.TLSServerName = strings.TrimSpace(check.TLSServerName)
	check.ServerName = strings.TrimSpace(check.ServerName)

	ctx := checkContext(groupName, index, check)
	labels, err := mergeLabels(ctx, cfg.Defaults.Labels, check.Labels)
	if err != nil {
		return err
	}
	check.Labels = labels
	if check.ID == "" {
		return fmt.Errorf("%s: id is required", ctx)
	}
	if check.Name == "" {
		return fmt.Errorf("%s: name is required", ctx)
	}
	if check.Kind == "" {
		return fmt.Errorf("%s: kind is required", ctx)
	}
	if check.Scope == "" {
		return fmt.Errorf("%s: scope is required", ctx)
	}
	if check.Interval.Duration < 0 {
		return fmt.Errorf("%s: interval must be non-negative", ctx)
	}
	if check.Timeout.Duration < 0 {
		return fmt.Errorf("%s: timeout must be non-negative", ctx)
	}
	if check.Interval.Duration == 0 {
		check.Interval = cfg.Defaults.Interval
	}
	if check.Timeout.Duration == 0 {
		check.Timeout = cfg.Defaults.Timeout
	}

	switch check.Kind {
	case "icmp":
		if check.Host == "" {
			return fmt.Errorf("%s: host is required for icmp", ctx)
		}
		if check.Count < 0 {
			return fmt.Errorf("%s: count must be non-negative", ctx)
		}
	case "http", "https":
		if check.URL == "" {
			return fmt.Errorf("%s: url is required for %s", ctx, check.Kind)
		}
		parsed, err := url.Parse(check.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("%s: url must be absolute with scheme and host", ctx)
		}
		if scheme := strings.ToLower(parsed.Scheme); scheme != check.Kind {
			return fmt.Errorf("%s: url scheme must be %q", ctx, check.Kind)
		}
		if check.Method == "" {
			check.Method = http.MethodGet
		} else {
			check.Method = strings.ToUpper(check.Method)
			if _, err := http.NewRequest(check.Method, parsed.String(), nil); err != nil {
				return fmt.Errorf("%s: method %q is invalid", ctx, check.Method)
			}
		}
		if len(check.ExpectedStatus) == 0 {
			check.ExpectedStatus = []int{200}
		}
		if err := validateExpectedStatus(check.ExpectedStatus); err != nil {
			return fmt.Errorf("%s: %w", ctx, err)
		}
	case "tcp":
		if check.Host == "" {
			return fmt.Errorf("%s: host is required for tcp", ctx)
		}
		if err := validatePort(check.Port); err != nil {
			return fmt.Errorf("%s: %w", ctx, err)
		}
	case "dns":
		if check.QueryName == "" {
			return fmt.Errorf("%s: query_name is required for dns", ctx)
		}
		if check.RecordType == "" {
			return fmt.Errorf("%s: record_type is required for dns", ctx)
		}
		if _, ok := dnsRecordTypes[check.RecordType]; !ok {
			return fmt.Errorf("%s: record_type %q is not supported", ctx, check.RecordType)
		}
		if check.Server == "" && cfg.Defaults.DNS.Server != "" {
			check.Server = cfg.Defaults.DNS.Server
		}
		if check.Port < 0 {
			return fmt.Errorf("%s: port must be non-negative", ctx)
		}
		if check.Port == 0 {
			check.Port = 53
		}
		if check.Port > 0 {
			if err := validatePort(check.Port); err != nil {
				return fmt.Errorf("%s: %w", ctx, err)
			}
		}
		if check.ExpectedRCode != "" {
			value, ok := rcodeMap[check.ExpectedRCode]
			if !ok {
				return fmt.Errorf("%s: expected_rcode %q is not supported", ctx, check.ExpectedRCode)
			}
			check.ExpectedRCodeValue = value
		}
	case "tls":
		if check.Host == "" {
			return fmt.Errorf("%s: host is required for tls", ctx)
		}
		if err := validatePort(check.Port); err != nil {
			return fmt.Errorf("%s: %w", ctx, err)
		}
		if check.MinDaysRemaining < 0 {
			return fmt.Errorf("%s: min_days_remaining must be non-negative", ctx)
		}
	default:
		return fmt.Errorf("%s: kind %q is not supported", ctx, check.Kind)
	}

	return nil
}

func validatePort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateExpectedStatus(statuses []int) error {
	for _, status := range statuses {
		if status < 100 || status > 599 {
			return fmt.Errorf("expected_status contains unsupported code %d", status)
		}
	}
	return nil
}

func checkContext(groupName string, index int, check *Check) string {
	if check.ID != "" {
		return fmt.Sprintf("group %q check %q", groupName, check.ID)
	}
	return fmt.Sprintf("group %q check[%d]", groupName, index)
}

func mergeLabels(ctx string, defaults map[string]string, overrides map[string]string) (map[string]string, error) {
	normalizedOverrides, err := normalizeLabels(ctx, overrides)
	if err != nil {
		return nil, err
	}
	if len(defaults) == 0 && len(normalizedOverrides) == 0 {
		return nil, nil
	}
	merged := make(map[string]string, len(defaults)+len(normalizedOverrides))
	for key, value := range defaults {
		merged[key] = value
	}
	for key, value := range normalizedOverrides {
		merged[key] = value
	}
	return merged, nil
}

func normalizeLabels(ctx string, labels map[string]string) (map[string]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string, len(labels))
	for key, value := range labels {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, fmt.Errorf("%s: label key is required", ctx)
		}
		if strings.HasPrefix(trimmedKey, "__") {
			return nil, fmt.Errorf("%s: label key %q is reserved", ctx, trimmedKey)
		}
		if !labelKeyPattern.MatchString(trimmedKey) {
			return nil, fmt.Errorf("%s: label key %q is invalid", ctx, trimmedKey)
		}
		if _, exists := normalized[trimmedKey]; exists {
			return nil, fmt.Errorf("%s: label key %q is duplicated", ctx, trimmedKey)
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	return normalized, nil
}
