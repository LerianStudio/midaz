// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package modulegraph guards the shape of the module graph itself.
//
// It exists because a Go major version lives in the import path, so two majors
// of the same library are two different sets of nominal types. When that
// happens to lib-observability the fleet gets a silent split brain: a logger or
// a tracer stored in a context.Context by one major is invisible to a
// NewLoggerFromContext from the other, and the second copy is dead weight in
// every binary.
package modulegraph

import (
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// obsModulePrefix is the module path of lib-observability, without any major suffix.
const obsModulePrefix = "github.com/LerianStudio/lib-observability"

// wantMajor is the ONE lib-observability major midaz builds against.
const wantMajor = "v4"

// extraMajorAllowlist names the modules that are permitted to drag a second
// lib-observability major into the build graph, each with the reason it is
// tolerated and the condition that removes the entry.
//
// This is an allowlist of IMPORTERS, not a relaxation of the rule: a second
// major reached from anywhere else — midaz's own packages included — still
// fails this test.
var extraMajorAllowlist = map[string]string{
	"github.com/LerianStudio/lib-service-discovery/v2": "lib-service-discovery v2.0.0 still requires lib-observability/v2. " +
		"INERT here: the library never reads telemetry out of a context.Context " +
		"(no NewLoggerFromContext, no ctx.Value lookup) — it is handed a logger " +
		"explicitly by the caller, so its v2 types never meet midaz's v4 ones. " +
		"REMOVE THIS ENTRY when LerianStudio/lib-service-discovery#33 ships and " +
		"lib-service-discovery is on lib-observability/v4.",
}

// listedPackage is the slice of `go list -json` this test reads.
type listedPackage struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
	Module     *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Module"`
}

var majorSuffix = regexp.MustCompile(`/v([0-9]+)$`)

// obsMajorOf returns the lib-observability major of a module path, or "" if the
// module is not lib-observability. The unsuffixed path is major v1.
func obsMajorOf(modulePath string) string {
	if modulePath == obsModulePrefix {
		return "v1"
	}

	if !strings.HasPrefix(modulePath, obsModulePrefix+"/v") {
		return ""
	}

	if m := majorSuffix.FindStringSubmatch(modulePath); m != nil {
		return "v" + m[1]
	}

	return ""
}

// repoRoot returns the module root, derived from this file's own location.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// buildTags are the build configurations the graph is checked under. The empty
// entry is the default build; "libsd" is the tag that compiles the
// lib-service-discovery adapter in pkg/servicediscovery, and is the ONLY
// configuration in which a second lib-observability major is reachable at all —
// so a guard that skipped it would be blind exactly where the allowlist below
// applies.
var buildTags = []string{"", "libsd"}

// buildGraph resolves the transitive non-test import graph of every midaz
// package under the given build tag and returns, per package, its module path
// and its direct imports.
func buildGraph(t *testing.T, tags string) map[string]listedPackage {
	t.Helper()

	args := []string{"list", "-deps", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}

	args = append(args, "./...")

	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot(t)

	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, isExit := err.(*exec.ExitError); isExit {
			stderr = string(ee.Stderr)
		}

		t.Fatalf("go list -deps failed: %v\n%s", err, stderr)
	}

	graph := make(map[string]listedPackage)
	dec := json.NewDecoder(strings.NewReader(string(out)))

	for {
		var pkg listedPackage

		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}

			t.Fatalf("decoding go list output: %v", err)
		}

		graph[pkg.ImportPath] = pkg
	}

	if len(graph) == 0 {
		t.Fatal("go list returned no packages")
	}

	return graph
}

// TestSingleLibObservabilityMajor fails when the build graph carries more than
// one major of lib-observability, unless every package of the extra major is
// reached exclusively from an allowlisted module.
func TestSingleLibObservabilityMajor(t *testing.T) {
	for _, tags := range buildTags {
		name := tags
		if name == "" {
			name = "default"
		}

		t.Run(name, func(t *testing.T) {
			assertSingleMajor(t, buildGraph(t, tags))
		})
	}
}

// assertSingleMajor is the check itself, run once per build configuration.
func assertSingleMajor(t *testing.T, graph map[string]listedPackage) {
	t.Helper()

	// moduleOf maps an import path to the module that provides it, and
	// majorPkgs collects the packages of each lib-observability major.
	moduleOf := make(map[string]string, len(graph))
	majorPkgs := make(map[string][]string)

	for path, pkg := range graph {
		if pkg.Module == nil {
			continue
		}

		moduleOf[path] = pkg.Module.Path

		if major := obsMajorOf(pkg.Module.Path); major != "" {
			majorPkgs[major] = append(majorPkgs[major], path)
		}
	}

	if _, present := majorPkgs[wantMajor]; !present {
		t.Fatalf("lib-observability/%s is not in the build graph at all; majors found: %v",
			wantMajor, sortedKeys(majorPkgs))
	}

	for _, major := range sortedKeys(majorPkgs) {
		if major == wantMajor {
			continue
		}

		// Every direct importer of a package of this extra major must belong to
		// an allowlisted module (or to the extra major itself, which is just its
		// own internal wiring).
		offenders := make(map[string][]string)
		extra := make(map[string]bool, len(majorPkgs[major]))

		for _, p := range majorPkgs[major] {
			extra[p] = true
		}

		for path, pkg := range graph {
			importerModule := moduleOf[path]
			if obsMajorOf(importerModule) == major {
				continue
			}

			if _, allowed := extraMajorAllowlist[importerModule]; allowed {
				continue
			}

			for _, imported := range pkg.Imports {
				if extra[imported] {
					offenders[path] = append(offenders[path], imported)
				}
			}
		}

		if len(offenders) > 0 {
			var b strings.Builder

			b.WriteString("lib-observability/" + major + " is reachable from outside the allowlist " +
				"(midaz builds against " + wantMajor + "):\n")

			for _, importer := range sortedKeys(offenders) {
				sort.Strings(offenders[importer])
				b.WriteString("  " + importer + " (module " + moduleOf[importer] + ") imports " +
					strings.Join(offenders[importer], ", ") + "\n")
			}

			b.WriteString("\nallowlisted importers:\n")

			for _, mod := range sortedKeys(extraMajorAllowlist) {
				b.WriteString("  " + mod + ": " + extraMajorAllowlist[mod] + "\n")
			}

			t.Error(b.String())
		}
	}
}

// sortedKeys returns the keys of m in deterministic order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
