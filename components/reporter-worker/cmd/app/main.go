// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/LerianStudio/midaz/v3/components/reporter-worker/internal/bootstrap"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
)

func main() {
	libCommons.InitLocalEnvConfig()

	svc, err := bootstrap.InitWorker()
	if err != nil {
		// fmt.Fprintf is used here because the structured logger (zap) is not yet
		// available — it is initialized inside InitWorker. This is the only place
		// where fmt output is acceptable per Ring standards.
		fmt.Fprintf(os.Stderr, "Failed to initialize worker: %v\n", err)
		os.Exit(1)
	}

	svc.Run()
}
