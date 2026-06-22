<table border="0" cellspacing="0" cellpadding="0">
  <tr>
    <td><img src="https://github.com/LerianStudio.png" width="72" alt="Lerian" /></td>
    <td><h1>Midaz</h1></td>
  </tr>
</table>

---

## Description

<!-- Summarize what this PR changes and why. -->

## Type of Change

- [ ] `feat`: New feature or capability
- [ ] `fix`: Bug fix
- [ ] `perf`: Performance improvement
- [ ] `refactor`: Internal restructuring with no behavior change
- [ ] `docs`: Documentation only
- [ ] `style`: Formatting, whitespace, naming (no logic change)
- [ ] `test`: Adding or updating tests
- [ ] `ci`: CI pipeline or workflow changes
- [ ] `build`: Build system, Dockerfile, Go module dependencies
- [ ] `chore`: Maintenance, config, tooling
- [ ] `revert`: Reverts a previous commit
- [ ] `BREAKING CHANGE`: Consumers must update their integration

## Breaking Changes

None.

## Testing

- [ ] `make test` passes
- [ ] `make test-int` passes if integration paths are exercised
- [ ] `make lint` passes
- [ ] `make sec` and `make vulncheck` pass

**Test evidence / Actions run:** <!-- Optional: link to a CI run or screenshot -->

## Architectural Checklist

- [ ] No `panic()` in production paths
- [ ] Timestamps use `time.Now().UTC()`
- [ ] Errors wrapped with `%w`
- [ ] Handlers stay thin (parse, validate, call service)
- [ ] Infrastructure concerns kept in `internal/bootstrap`

## Related Issues

Closes #
