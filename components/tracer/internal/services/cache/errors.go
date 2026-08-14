// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

import "errors"

var (
	// ErrNilCache is returned when a nil cache is provided.
	ErrNilCache = errors.New("cache cannot be nil")

	// ErrNilRepository is returned when a nil repository is provided.
	ErrNilRepository = errors.New("repository cannot be nil")

	// ErrNilCompiler is returned when a nil expression compiler is provided.
	ErrNilCompiler = errors.New("expression compiler cannot be nil")

	// ErrNilLogger is returned when a nil logger is provided.
	ErrNilLogger = errors.New("logger cannot be nil")
)
