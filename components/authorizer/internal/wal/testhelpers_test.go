// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package wal

// testHMACKey is a fixed 32-byte key used across the wal package's tests so
// that frames written by one test helper can be replayed by another. It is
// intentionally NOT the production key and never leaves tests.
var testHMACKey = []byte("wal-test-hmac-key-thirty-2bytes!")
