// Package buildinfo exposes the build provenance (commit, build time, dirty
// flag) of the running binary, sourced from -ldflags overrides or the Go
// runtime debug.BuildInfo VCS stamps.
package buildinfo

import (
	"runtime/debug"
	"sync"
)

// commit and buildTime are overridable at link time via:
//
//	-X github.com/LerianStudio/midaz/v4/pkg/buildinfo.commit=<sha>
//	-X github.com/LerianStudio/midaz/v4/pkg/buildinfo.buildTime=<rfc3339>
//
// When empty (the default), values fall back to the runtime VCS stamps.
var (
	commit    string
	buildTime string
)

// Info holds the build provenance of the running binary.
type Info struct {
	Commit    string
	BuildTime string
	Dirty     bool
}

var (
	once   sync.Once
	cached Info
)

// Get returns the memoized build provenance of the running binary.
func Get() Info {
	once.Do(func() {
		bi, _ := debug.ReadBuildInfo()
		cached = compute(bi, commit, buildTime)
	})

	return cached
}

// compute resolves build provenance with per-field precedence:
// ldflags value (non-empty) > VCS stamp > "unknown" (Commit/BuildTime) or
// false (Dirty). Dirty has no ldflags override since an ldflags build is a
// controlled build.
func compute(bi *debug.BuildInfo, ldCommit, ldBuildTime string) Info {
	info := Info{Commit: "unknown", BuildTime: "unknown"}

	if bi != nil {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if s.Value != "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if s.Value != "" {
					info.BuildTime = s.Value
				}
			case "vcs.modified":
				info.Dirty = s.Value == "true"
			}
		}
	}

	if ldCommit != "" {
		info.Commit = ldCommit
	}

	if ldBuildTime != "" {
		info.BuildTime = ldBuildTime
	}

	return info
}
