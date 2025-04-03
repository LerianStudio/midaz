# Midaz Scripts

This directory contains helper scripts used by the Makefile to perform various tasks in the Midaz project.

## Overview

These scripts help simplify the Makefile by moving complex command logic to dedicated shell scripts. This approach has several benefits:

1. **Improved Readability**: The Makefile becomes more concise and easier to understand
2. **Better Maintainability**: Complex logic is isolated in dedicated scripts that can be tested independently
3. **Easier Debugging**: Shell scripts are easier to debug than Makefile commands
4. **Reusability**: Scripts can be called from other places, not just the Makefile

## Available Scripts

### Build System Scripts

- **show_help.sh**: Displays help information for the Makefile targets
- **run_sdk_command.sh**: Runs commands in the SDK directory
- **run_tests.sh**: Runs tests for both the main project and the Go SDK

### Code Quality Scripts

- **run_lint.sh**: Runs linters on all components and the Go SDK
- **run_format.sh**: Formats code in all components and the Go SDK
- **regenerate_mocks.sh**: Regenerates mock files for all components and the Go SDK
- **cleanup_mocks.sh**: Cleans up existing mock files
- **cleanup_regenerate_mocks.sh**: Combines cleanup and regeneration of mock files

### Verification Scripts

- **check-logs.sh**: Verifies error logging patterns in the codebase
- **check-envs.sh**: Checks environment variables in the codebase
- **check-tests.sh**: Verifies test coverage in the codebase

### Lint Fixing Scripts

- **fix_case_whitespace.sh**: Fixes case statement whitespace issues in specific files
- **fix_block_whitespace.sh**: Fixes "block should not start with a whitespace" issues in Go files
- **fix_wsl_issues.sh**: Fixes common whitespace linter (wsl) issues in Go code
- **fix_lint_issues.sh**: Automatically fixes common lint issues across the Midaz codebase

## Common Linting Issues and Fixes

The `run_lint.sh` script can detect various issues in the codebase. Here are some common issues and how they were fixed:

1. **Unnecessary nil checks**:
   - Issue: Checking if a map is nil before ranging over it is unnecessary in Go
   - Fix: Remove the nil check as Go handles nil maps safely in range statements

2. **Unnecessary type conversions**:
   - Issue: Converting a value to the same type it already is
   - Fix: Remove the redundant type conversion

3. **High complexity functions**:
   - Issue: Functions with high cognitive or cyclomatic complexity
   - Fix: Refactor by extracting logic into smaller, focused helper functions

4. **Whitespace issues (wsl)**:
   - Issue: Improper spacing between declarations or statements
   - Fix: Add appropriate blank lines between variable declarations or statements

## Best Practices

1. **Run `make lint` before committing**: This will catch and fix most common issues
2. **Run `make format` to ensure consistent formatting**: This applies standard Go formatting
3. **Address complex issues manually**: Refactor complex functions to reduce complexity
4. **Keep the codebase clean**: Regular linting helps maintain code quality

## Usage Examples

### Running Linters

```bash
# Run linters on all components and the Go SDK
./scripts/run_lint.sh

# Or use the Makefile target
make lint
```

### Formatting Code

```bash
# Format code in all components and the Go SDK
./scripts/run_format.sh

# Or use the Makefile target
make format
```

### Regenerating Mocks

```bash
# Regenerate mock files for all components and the Go SDK
./scripts/regenerate_mocks.sh

# Or use the Makefile target
make regenerate-mocks
```

## Contributing

When adding new scripts to this directory:

1. Make sure they follow the same pattern as existing scripts
2. Use proper error handling and variable management
3. Add appropriate documentation in this README
4. Update the Makefile to use the new script if needed
