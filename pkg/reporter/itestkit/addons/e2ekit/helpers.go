package e2ekit

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
)

var (
	projectRootOnce sync.Once
	projectRootPath string
)

// ProjectRoot returns the root directory of the project by walking up from this
// file's location until it finds a go.mod file. The result is cached.
//
// This is useful for building images from Dockerfiles when the test is in a
// subdirectory of the project.
//
// Example:
//
//	app, err := e2ekit.New(t).
//	    WithDockerfile(e2ekit.BuildConfig{
//	        ContextDir: e2ekit.ProjectRoot(),
//	        Dockerfile: "components/api/Dockerfile",
//	    }).
//	    Run()
func ProjectRoot() string {
	projectRootOnce.Do(func() {
		// Use runtime.Caller(0) to get this file's location, then walk up to find go.mod.
		// This works because this file is inside the project (pkg/itestkit/addons/e2ekit/).
		_, file, _, _ := runtime.Caller(0)

		dir := filepath.Dir(file)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				projectRootPath = dir
				return
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}

			dir = parent
		}
	})

	return projectRootPath
}

// ProjectRootFrom returns the project root by walking up from the given starting path.
// Unlike ProjectRoot(), this is not cached and always computes the result.
//
// Use this when you need to find the project root from a specific location
// rather than the caller's location.
func ProjectRootFrom(startPath string) string {
	dir := startPath
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return ""
}

// HostGatewayIP returns the address that containers should use to reach the host.
// This is used to connect from containers to services running on the host.
// The result is cached.
//
// On Docker Desktop, returns "host.docker.internal" which is the standard hostname.
// On Linux with default bridge, returns the bridge gateway IP (typically 172.17.0.1).
//
// Deprecated: Use itestkit.HostGatewayIP() directly. This function is kept for
// backward compatibility.
func HostGatewayIP() string {
	return itestkit.HostGatewayIP()
}
