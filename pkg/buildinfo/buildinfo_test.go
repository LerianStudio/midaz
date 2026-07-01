package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestCompute(t *testing.T) {
	tests := []struct {
		name         string
		bi           *debug.BuildInfo
		ldCommit     string
		ldBuildTime  string
		wantCommit   string
		wantBuildTim string
		wantDirty    bool
	}{
		{
			name: "ldflags set and vcs settings present: ldflags win, dirty from vcs",
			bi: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "vcsrev"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			}},
			ldCommit:     "ldrev",
			ldBuildTime:  "2026-06-06T12:00:00Z",
			wantCommit:   "ldrev",
			wantBuildTim: "2026-06-06T12:00:00Z",
			wantDirty:    true,
		},
		{
			name: "vcs settings only: vcs values, dirty true",
			bi: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "vcsrev"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			}},
			wantCommit:   "vcsrev",
			wantBuildTim: "2026-01-01T00:00:00Z",
			wantDirty:    true,
		},
		{
			name:         "nil BuildInfo: unknown/unknown/false",
			bi:           nil,
			wantCommit:   "unknown",
			wantBuildTim: "unknown",
			wantDirty:    false,
		},
		{
			name:         "BuildInfo without vcs settings: unknown/unknown/false",
			bi:           &debug.BuildInfo{Settings: []debug.BuildSetting{}},
			wantCommit:   "unknown",
			wantBuildTim: "unknown",
			wantDirty:    false,
		},
		{
			name: "modified false: dirty false",
			bi: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "vcsrev"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
			}},
			wantCommit:   "vcsrev",
			wantBuildTim: "2026-01-01T00:00:00Z",
			wantDirty:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compute(tt.bi, tt.ldCommit, tt.ldBuildTime)
			if got.Commit != tt.wantCommit {
				t.Errorf("Commit = %q, want %q", got.Commit, tt.wantCommit)
			}
			if got.BuildTime != tt.wantBuildTim {
				t.Errorf("BuildTime = %q, want %q", got.BuildTime, tt.wantBuildTim)
			}
			if got.Dirty != tt.wantDirty {
				t.Errorf("Dirty = %v, want %v", got.Dirty, tt.wantDirty)
			}
		})
	}
}
