// Package version reports the envseal build version.
package version

import (
	"runtime/debug"
	"sync"
)

// Version is set at build time with:
//
//	-ldflags "-X github.com/PeacexF/envseal/internal/version.Version=v1.2.3"
var Version = "dev"

var describe = sync.OnceValue(func() string {
	if Version != "dev" {
		return Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}

	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision == "" {
		return Version
	}

	v := Version + "-" + revision[:min(len(revision), 12)]
	if modified == "true" {
		v += "-dirty"
	}
	return v
})

// String returns the version for display: "dev-<revision>[-dirty]"
// for untagged builds from a Git checkout.
func String() string { return describe() }
