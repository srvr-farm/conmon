package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReturnsNonZeroOnMissingConfig(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"-config", "/this/path/does/not/exist.yml"}, &stderr)
	if code == 0 {
		t.Fatalf("run returned %d, want non-zero", code)
	}
	if !strings.Contains(stderr.String(), "failed to load config:") {
		t.Fatalf("stderr = %q, want load-config error", stderr.String())
	}
}
