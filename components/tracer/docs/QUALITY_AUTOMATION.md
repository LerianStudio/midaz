# Quality Automation - CI + Makefile

This document explains the automated quality checks using **Makefile** (local) and **GitHub Actions** (CI).

---

## Quick Start

### Before Committing (Local)

Run quality checks locally for fast feedback:

```bash
make quality
```

**What it does:**
- ✅ Runs golangci-lint (errorlint, contextcheck)
- ✅ Verifies build tags on test files
- ✅ Runs all unit tests

**Time:** ~10-30 seconds (after first run)

### Automated (CI)

Every PR and push to `main`/`develop` automatically runs:
- ✅ Linting
- ✅ Build tags verification
- ✅ Unit tests
- ✅ Test determinism (3x runs)

**No setup required** - runs automatically on GitHub.

---

## Makefile Targets

### `make quality` (Recommended before commit)

Runs all quality checks in one command:

```bash
make quality
```

**Output if successful:**
```
✅ Linting completed successfully
✅ All test files have build tags
✅ Unit tests passed

Ready to commit and push!
```

**Output if failed:**
```
❌ ERROR: Test files missing //go:build tags:
./internal/some_test.go

All test files must have: //go:build unit
See: docs/CODING_STANDARDS.md section 3
```

### Individual Checks

Run checks separately:

```bash
# Linting only
make lint

# Build tags only
make check-build-tags

# Unit tests only
make test

# Integration tests
make test-integration
```

---

## GitHub Actions Workflows

### Workflow: Code Quality

**File:** `.github/workflows/code-quality.yml`

**Triggers:**
- Pull requests to `main` or `develop`
- Direct pushes to `main` or `develop`

**Jobs:**

1. **lint** - Runs golangci-lint with all configured rules
2. **build-tags** - Verifies all `*_test.go` have `//go:build unit`
3. **test** - Runs unit tests once
4. **test-determinism** - Runs unit tests 3x to verify no flaky tests

**Duration:** ~2-5 minutes total (jobs run in parallel)

### Viewing Results

On GitHub PR page:

1. Scroll to **"Checks"** section at bottom
2. See status of each job:
   - ✅ Lint (errorlint, contextcheck)
   - ✅ Verify Build Tags
   - ✅ Unit Tests
   - ✅ Test Determinism (3x runs)

3. Click job name to see details if failed

**PR cannot be merged until all checks pass.**

---

## Development Workflow

### Recommended Flow

```bash
# 1. Write code...

# 2. Before committing, run local checks
make quality

# 3. If all pass, commit and push
git add .
git commit -m "feat: add feature X"
git push

# 4. Create PR on GitHub
# → CI runs automatically
# → Wait for green checks before merging
```

### If Local Checks Fail

**Linting errors:**
```bash
# Run lint to see details
make lint

# Fix issues manually or let golangci-lint auto-fix
make lint

# Verify fixes
git diff

# Commit fixes
git add .
git commit -m "fix: address linting issues"
```

**Missing build tags:**
```bash
# See which files are missing tags
make check-build-tags

# Add to each missing file (after copyright header):
//go:build unit

package yourpackage

# Verify
make check-build-tags
```

**Test failures:**
```bash
# Run tests to see failures
make test

# Fix issues
# ...

# Verify
make test
```

---

## CI Failures

### "Lint failed"

**Cause:** Code doesn't meet linting standards

**Fix:**
```bash
# Run locally to see issues
make lint

# Fix and push
git add .
git commit -m "fix: address linting issues"
git push
```

### "Build tags check failed"

**Cause:** Test files missing `//go:build unit`

**Fix:**
```bash
# Check locally
make check-build-tags

# Add tags to missing files
# Then push
git push
```

### "Tests failed"

**Cause:** Unit tests failing

**Fix:**
```bash
# Run locally
make test

# Fix failing tests
# Then push
git push
```

### "Test determinism failed"

**Cause:** Flaky tests (non-deterministic)

**Common causes:**
- Using `time.Now()` in tests → use `testutil.FixedTime()`
- Using `uuid.New()` in tests → use `testutil.MustDeterministicUUID()`
- Race conditions
- Order dependencies

**Fix:**
```bash
# Run tests multiple times locally
for i in {1..10}; do go test ./... || break; done

# Identify flaky test
# Fix by using deterministic values
# See: docs/CODING_STANDARDS.md section 3
```

---

## Bypassing Checks (NOT RECOMMENDED)

### Local Checks

You can skip `make quality` and commit directly, but **CI will still catch issues**.

**Not recommended:** Wastes time waiting for CI to fail.

### CI Checks

**Cannot be bypassed.** PR requires all checks to pass before merge.

**Emergency bypass (admin only):**
- Repository admins can override and force merge
- Should only be used in true emergencies
- Requires approval from tech lead

---

## Comparison: Local vs CI

| Aspect | make quality (Local) | GitHub Actions (CI) |
|--------|---------------------|---------------------|
| **Speed** | ⚡ Fast (10-30s) | ⏱️ Slower (2-5min) |
| **Mandatory** | ❌ Optional | ✅ Required |
| **Setup** | ✅ None (just make) | ✅ None |
| **Offline** | ✅ Works offline | ❌ Needs GitHub |
| **Feedback** | 🚀 Immediate | ⏳ After push |
| **Can bypass** | ✅ Yes | ❌ No |

**Recommendation:** Use BOTH
- `make quality` for fast local feedback
- CI for enforcement gate

---

## Troubleshooting

### "make: command not found"

Install make:
```bash
# macOS (should be installed by default)
xcode-select --install

# Linux
sudo apt install build-essential  # Ubuntu/Debian
```

### "golangci-lint: command not found"

Will auto-install on first `make lint`, or install manually:
```bash
# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

### "make quality is slow"

**First run:** Always slow (downloads dependencies)

**Subsequent runs:** Should be fast (~10-30s)

**If still slow:**
```bash
# Clear cache
go clean -cache -testcache

# Try again
make quality
```

### CI stuck/hanging

**Symptoms:** Job runs for >10 minutes

**Cause:** Usually test hanging or infinite loop

**Fix:**
```bash
# Run tests locally with timeout
go test -timeout 30s ./...

# Identify hanging test
# Add timeout or fix issue
```

---

## Customization

### Adding New Checks

Edit `Makefile`:

```makefile
.PHONY: my-custom-check
my-custom-check:
	@echo "Running my check..."
	# Your check here
	@echo "✅ Check passed"

.PHONY: quality
quality: lint check-build-tags test my-custom-check
	# ...
```

Edit `.github/workflows/code-quality.yml`:

```yaml
jobs:
  my-custom-check:
    name: My Custom Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run check
        run: make my-custom-check
```

### Disabling Checks

**Local (Makefile):**

Comment out from `quality` target:

```makefile
.PHONY: quality
quality: lint test  # Removed: check-build-tags
```

**CI:**

Comment out job in `.github/workflows/code-quality.yml`:

```yaml
# build-tags:  # ← Disabled
#   name: Verify Build Tags
#   ...
```

---

## Best Practices

### DO:
- ✅ Run `make quality` before committing
- ✅ Fix issues locally (faster than waiting for CI)
- ✅ Check CI status before requesting review
- ✅ Investigate flaky tests immediately

### DON'T:
- ❌ Push without running `make quality` first
- ❌ Ignore CI failures ("it passed locally")
- ❌ Request review with failing CI checks
- ❌ Commit flaky/non-deterministic tests

---

## FAQ

### Q: Do I have to run `make quality` before every commit?

**A:** Not mandatory, but **strongly recommended**. CI will catch issues anyway, but local checks give faster feedback.

### Q: What if `make quality` passes but CI fails?

**A:** Usually due to:
- Different Go version (update local Go)
- Cached results (run `go clean -cache`)
- Environment differences (check CI logs)

**Solution:** Match local environment to CI (Go 1.22, clean cache).

### Q: Can I skip checks for work-in-progress commits?

**A:** Locally yes, but don't push until checks pass. Use local branches:

```bash
# WIP commit (skip checks)
git commit -m "wip: work in progress"

# Before pushing, clean up commits and verify
make quality
git rebase -i HEAD~3  # Clean up commits
git push
```

### Q: How do I know which check failed in CI?

**A:** On PR page:
1. Click failing check name
2. Expand failed step
3. Read error message
4. Fix locally and push

---

## See Also

- [CODING_STANDARDS.md](./CODING_STANDARDS.md) - Standards enforced by checks
- [LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md) - English-only policy
- [PROJECT_RULES.md](./PROJECT_RULES.md) - Architecture and conventions

---

**Last Updated:** 2026-02-04  
**Version:** 1.0  
**Maintained by:** Engineering Team
