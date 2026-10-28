package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set at build time through -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionString describes this build for the --version flag.
func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}

	return fmt.Sprintf("gdrift %s (%s, %s, %s)", v, commit, date, runtime.Version())
}
