package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	got := versionString()
	if !strings.HasPrefix(got, "gdrift dev") {
		t.Errorf("versionString() = %q, want prefix %q", got, "gdrift dev")
	}

	if !strings.Contains(got, runtime.Version()) {
		t.Errorf("versionString() = %q, want it to contain %q", got, runtime.Version())
	}
}
