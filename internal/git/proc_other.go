//go:build !unix

package git

import "os/exec"

// isolate keeps the default process handling where sessions are unavailable.
func isolate(_ *exec.Cmd) (release func()) { return func() {} }
