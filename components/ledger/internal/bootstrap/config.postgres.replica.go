// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"
	"strings"
)

// errIncompleteReplicaConfig is returned when only some of the six
// DB_<MODULE>_REPLICA_* variables are set. The ledger refuses to start rather
// than silently serving reads from the primary while the operator believes a
// replica is in use.
var errIncompleteReplicaConfig = errors.New("postgres replica configuration is incomplete")

// buildOptionalReplicaDSN decides how a module's read-replica settings reach
// lib-commons:
//
//   - all six values blank (after trimming whitespace): returns "", which
//     lib-commons v7.1.1+ reads as "no replica" and serves reads from the
//     primary pool;
//   - all six values set: returns the DSN in the same format used for the
//     primary, byte-identical to the legacy builders (values are not trimmed);
//   - anything in between: returns errIncompleteReplicaConfig naming the module
//     and the missing fields. A half-configured replica is never downgraded to
//     primary-only, because the operator would not find out.
//
// module is the constant.Module* name; it selects the DB_<MODULE>_REPLICA_*
// variables named in the error.
func buildOptionalReplicaDSN(module, host, user, password, dbname, port, sslmode string) (string, error) {
	fields := []struct {
		name  string
		value string
	}{
		{name: "host", value: host},
		{name: "user", value: user},
		{name: "password", value: password},
		{name: "dbname", value: dbname},
		{name: "port", value: port},
		{name: "sslmode", value: sslmode},
	}

	missing := make([]string, 0, len(fields))

	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name)
		}
	}

	switch len(missing) {
	case len(fields):
		return "", nil
	case 0:
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
			host, user, password, dbname, port, sslmode), nil
	default:
		return "", fmt.Errorf("%w for module %q: missing: %s (set every DB_%s_REPLICA_* variable to use a replica, or none to use a single pool on the primary)",
			errIncompleteReplicaConfig, module, strings.Join(missing, ", "), strings.ToUpper(module))
	}
}
