// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import "regexp"

type Pattern struct {
	Regex      *regexp.Regexp
	Message    string
	Severity   string
	ExcludeIf  *regexp.Regexp
	Suggestion string
}

var dangerousPatterns = []Pattern{
	{
		Regex:    regexp.MustCompile(`(?i)DROP\s+COLUMN`),
		Message:  "DROP COLUMN detected. Use expand-and-contract pattern in separate releases.",
		Severity: "ERROR",
		Suggestion: `Use expand-and-contract pattern:
    1. Create new column (if replacing)
    2. Deploy app writing to both columns
    3. Migrate existing data
    4. Deploy app reading from new column
    5. DROP COLUMN in a future release after deprecation period`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)ALTER\s+(TABLE\s+\w+\s+)?ALTER\s+COLUMN\s+\w+\s+(SET\s+DATA\s+)?TYPE`),
		Message:  "ALTER COLUMN TYPE detected. Add new column, migrate data, then remove old one in separate release.",
		Severity: "ERROR",
		Suggestion: `Use expand-and-contract pattern:
    1. ADD COLUMN new_column NEW_TYPE
    2. Deploy app writing to both columns
    3. UPDATE table SET new_column = old_column::NEW_TYPE
    4. Deploy app reading from new_column
    5. DROP COLUMN old_column in a future release`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)VACUUM\s+FULL`),
		Message:  "VACUUM FULL requires exclusive lock. Use VACUUM or VACUUM ANALYZE.",
		Severity: "ERROR",
		Suggestion: `Replace with non-blocking alternatives:
    - VACUUM table_name;
    - VACUUM ANALYZE table_name;
    - Consider pg_repack for reclaiming space without exclusive locks`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)TRUNCATE\s+`),
		Message:  "TRUNCATE destroys all data. Use soft delete or archiving.",
		Severity: "ERROR",
		Suggestion: `Use safer alternatives:
    - DELETE FROM table WHERE condition;
    - UPDATE table SET deleted_at = NOW() WHERE condition;
    - Move data to archive table before deletion`,
	},
	{
		Regex:     regexp.MustCompile(`(?i)DROP\s+TABLE\s+`),
		ExcludeIf: regexp.MustCompile(`(?i)DROP\s+TABLE\s+IF\s+EXISTS`),
		Message:   "DROP TABLE without IF EXISTS may fail. Use DROP TABLE IF EXISTS.",
		Severity:  "WARNING",
		Suggestion: `Add IF EXISTS clause:
    DROP TABLE IF EXISTS table_name;`,
	},
	{
		Regex:     regexp.MustCompile(`(?i)REINDEX\s+`),
		ExcludeIf: regexp.MustCompile(`(?i)REINDEX\s+(TABLE\s+)?CONCURRENTLY`),
		Message:   "REINDEX without CONCURRENTLY blocks table. Use REINDEX CONCURRENTLY.",
		Severity:  "WARNING",
		Suggestion: `Use CONCURRENTLY option (PostgreSQL 12+):
    REINDEX TABLE CONCURRENTLY table_name;
    REINDEX INDEX CONCURRENTLY index_name;`,
	},
	{
		Regex:     regexp.MustCompile(`(?i)CREATE\s+(UNIQUE\s+)?INDEX\s+`),
		ExcludeIf: regexp.MustCompile(`(?i)CREATE\s+(UNIQUE\s+)?INDEX\s+(CONCURRENTLY|IF)`),
		Message:   "CREATE INDEX without CONCURRENTLY blocks table. Use CREATE INDEX CONCURRENTLY.",
		Severity:  "WARNING",
		Suggestion: `Use CONCURRENTLY option:
    CREATE INDEX CONCURRENTLY idx_name ON table (column);
    CREATE UNIQUE INDEX CONCURRENTLY idx_name ON table (column);
    Note: Cannot be used inside a transaction block`,
	},
	{
		Regex:     regexp.MustCompile(`(?i)DROP\s+INDEX\s+`),
		ExcludeIf: regexp.MustCompile(`(?i)DROP\s+INDEX\s+(CONCURRENTLY|IF)`),
		Message:   "DROP INDEX without CONCURRENTLY blocks table. Use DROP INDEX CONCURRENTLY.",
		Severity:  "WARNING",
		Suggestion: `Use CONCURRENTLY option:
    DROP INDEX CONCURRENTLY IF EXISTS idx_name;
    Note: Cannot be used inside a transaction block`,
	},
	{
		Regex:     regexp.MustCompile(`(?i)ADD\s+COLUMN\s+\w+\s+\w+.*\s+NOT\s+NULL`),
		ExcludeIf: regexp.MustCompile(`(?i)(NOT\s+NULL\s+DEFAULT|DEFAULT\s+\S+.*NOT\s+NULL)`),
		Message:   "Adding NOT NULL column without DEFAULT requires table rewrite. Add DEFAULT value.",
		Severity:  "ERROR",
		Suggestion: `Add DEFAULT value to avoid table rewrite:
    ALTER TABLE table_name ADD COLUMN column_name TYPE NOT NULL DEFAULT value;

    Or use multi-step approach:
    1. ADD COLUMN column_name TYPE DEFAULT value;
    2. UPDATE table SET column_name = value WHERE column_name IS NULL;
    3. ALTER COLUMN column_name SET NOT NULL;`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)RENAME\s+COLUMN`),
		Message:  "RENAME COLUMN breaks existing queries. Use expand-and-contract: add new column, migrate, remove old.",
		Severity: "ERROR",
		Suggestion: `Use expand-and-contract pattern:
    1. ADD COLUMN new_name TYPE;
    2. UPDATE table SET new_name = old_name;
    3. Deploy app using new_name
    4. DROP COLUMN old_name in a future release

    Or create a view for backward compatibility`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)RENAME\s+TABLE`),
		Message:  "RENAME TABLE breaks existing queries. Create new table, migrate data, remove old.",
		Severity: "ERROR",
		Suggestion: `Use expand-and-contract pattern:
    1. CREATE TABLE new_name (LIKE old_name INCLUDING ALL);
    2. Create triggers to sync data between tables
    3. Deploy app using new_name
    4. DROP TABLE old_name in a future release

    Or create a view with the old name pointing to new table`,
	},
	{
		Regex:    regexp.MustCompile(`(?i)ALTER\s+(TABLE\s+\w+\s+)?ALTER\s+COLUMN\s+\w+\s+SET\s+NOT\s+NULL`),
		Message:  "SET NOT NULL may fail if NULLs exist and blocks table for validation.",
		Severity: "WARNING",
		Suggestion: `Use constraint-based approach for large tables:
    1. First ensure no NULLs exist:
       UPDATE table SET column = default_value WHERE column IS NULL;

    2. Add constraint as NOT VALID (non-blocking):
       ALTER TABLE table ADD CONSTRAINT column_not_null
       CHECK (column IS NOT NULL) NOT VALID;

    3. Validate in separate transaction:
       ALTER TABLE table VALIDATE CONSTRAINT column_not_null;`,
	},
}

type Issue struct {
	File       string
	Line       int
	Severity   string
	Message    string
	Suggestion string
}
