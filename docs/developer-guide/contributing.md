# Contributing Guide

**Navigation:** [Home](../../) > [Developer Guide](../) > Contributing

This document provides comprehensive guidelines for developers who want to contribute to the Midaz project. It outlines our development workflow, coding standards, testing requirements, and pull request process.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Testing Requirements](#testing-requirements)
- [Submitting Pull Requests](#submitting-pull-requests)
- [Commit Message Guidelines](#commit-message-guidelines)
- [Code Review Process](#code-review-process)
- [Documentation Requirements](#documentation-requirements)
- [Issue Tracking](#issue-tracking)
- [Release Process](#release-process)
- [Community Guidelines](#community-guidelines)

## Getting Started

### Prerequisites

Before contributing to Midaz, ensure you have the following tools installed:

- Go 1.20 or higher
- Docker and Docker Compose
- Git (with commit signing configured)
- Make

### Setting Up Your Development Environment

1. **Fork and Clone the Repository**

   ```bash
   git clone https://github.com/YOUR-USERNAME/midaz.git
   cd midaz
   git remote add upstream https://github.com/LerianStudio/midaz.git
   ```

2. **Install Dependencies**

   ```bash
   make setup
   ```

3. **Set Up Git Hooks**

   ```bash
   make setup-git-hooks
   ```

4. **Start the Development Environment**

   ```bash
   make dev
   ```

## Development Workflow

Midaz follows a feature branch workflow with pull requests:

1. **Create an Issue**: For most changes, start by creating an issue in the GitHub repository to discuss your proposed change. This isn't necessary for minor fixes like typos.

2. **Create a Branch**: Create a feature or fix branch from the `main` branch with a descriptive name:

   ```bash
   git checkout main
   git pull upstream main
   git checkout -b feature/your-feature-name
   ```

   Branch naming conventions:
   - `feature/`: For new features
   - `fix/`: For bug fixes
   - `docs/`: For documentation changes
   - `refactor/`: For code refactoring
   - `test/`: For adding or modifying tests

3. **Make Changes**: Implement your changes, following our coding standards and ensuring all tests pass.

4. **Commit Changes**: Make frequent, small commits with clear messages following our commit guidelines.

5. **Push to Your Fork**:

   ```bash
   git push origin feature/your-feature-name
   ```

6. **Create a Pull Request**: Submit a pull request to the `main` branch of the main repository.

7. **Code Review**: Address any feedback from code reviewers.

8. **Merge**: Once approved, your pull request will be merged by a maintainer.

## Coding Standards

### Go Style Guidelines

Midaz adheres to the standard Go style guidelines and best practices:

- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Adhere to [Effective Go](https://golang.org/doc/effective_go.html) principles
- Run `gofmt` and `golint` on your code before committing

### Architecture Guidelines

When contributing code to Midaz, ensure your changes align with our architectural patterns:

- **Hexagonal Architecture**: Maintain the separation between domain, application, and infrastructure layers
- **CQRS Pattern**: Keep command (write) and query (read) operations separate
- **Clean Code**: Maintain high readability and maintainability standards
- **Dependency Injection**: Use dependency injection for service components

### Package Structure

Respect the existing package structure:

- `/components/*`: Service components (onboarding, transaction, mdz CLI)
- `/pkg/*`: Shared packages used across components
- `/internal/*`: Component-specific internal implementation

## Testing Requirements

All code contributions must include appropriate tests:

### Unit Tests

- Write unit tests for all new functions and methods
- Aim for at least 75% code coverage for unit tests
- Use table-driven tests where appropriate
- Use mocks for external dependencies

### Integration Tests

- Add integration tests for API endpoints and database interactions
- Use the testing infrastructure in the `test/integration` directory

### Running Tests

Before submitting a pull request, ensure all tests pass:

```bash
make test       # Run unit tests
make test-int   # Run integration tests
make coverage   # Generate coverage report
```

## Submitting Pull Requests

1. **Ensure your branch is up-to-date**:

   ```bash
   git checkout main
   git pull upstream main
   git checkout your-branch-name
   git rebase main
   ```

2. **Verify your changes**:
   - All tests pass
   - No linting errors
   - Your code meets our standards
   - Documentation is updated if applicable

3. **Create a pull request** with a clear description:
   - Reference the related issue(s)
   - Describe what the change does
   - Explain why the change is necessary
   - Include any breaking changes or backward compatibility issues
   - Document any new dependencies

4. **Sign your commits**:
   - All commits must be signed to verify you have the right to submit the code
   - Configure Git to sign commits:
     ```bash
     git config --global user.name "Your Name"
     git config --global user.email "your.email@example.com"
     git config --global commit.gpgsign true   # If using GPG
     ```
   - Or use the `-s` flag to add a Signed-off-by line:
     ```bash
     git commit -s -m "Your commit message"
     ```

## Commit Message Guidelines

Midaz uses the [Conventional Commits](https://www.conventionalcommits.org/) specification for commit messages to ensure a clear and standardized history:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types

- **fix**: A bug fix (correlates with PATCH in SemVer)
- **feat**: A new feature (correlates with MINOR in SemVer)
- **docs**: Documentation changes
- **style**: Code style changes (formatting, etc.; no code change)
- **refactor**: Code changes that neither fix bugs nor add features
- **perf**: Performance improvements
- **test**: Adding or correcting tests
- **chore**: Changes to the build process, tools, etc.

### Breaking Changes

Indicate breaking changes with either:
- A `!` after the type/scope: `feat!: introduce breaking API change`
- A footer: `BREAKING CHANGE: description of the breaking change`

### Examples

```
feat(transaction): add support for multi-currency operations

Implement ability to process transactions in multiple currencies
with automatic conversion using latest exchange rates.

Resolves: #123
```

```
fix(api): correct error handling in account creation endpoint

BREAKING CHANGE: Error response format has changed to align with
API standards across the platform.
```

## Code Review Process

1. **Initial Review**: A project maintainer will review your pull request for basic completeness
2. **Automated Checks**: CI/CD pipeline will run tests, linting, and other checks
3. **Detailed Review**: Maintainers will review code for quality, correctness, and architectural fit
4. **Feedback**: Address any feedback from reviewers
5. **Approval and Merge**: Once approved, a maintainer will merge your pull request

### Review Criteria

Pull requests are evaluated based on:
- Code quality and correctness
- Test coverage
- Documentation completeness
- Adherence to coding standards
- Architectural alignment
- Performance implications
- Security considerations

## Documentation Requirements

All code contributions should include appropriate documentation:

1. **Code Comments**: Add clear comments for complex logic or algorithms
2. **API Documentation**: Update or add documentation for public APIs
3. **User-Facing Documentation**: Update user documentation for new features
4. **Examples**: Include examples for new functionality where appropriate

## Issue Tracking

### Creating Issues

When creating a new issue:
- Use a clear, descriptive title
- Provide a detailed description with steps to reproduce for bugs
- Include screenshots or logs if applicable
- Add appropriate labels (bug, enhancement, documentation, etc.)
- Link to related issues or PRs

### Issue Labels

Midaz uses the following primary issue labels:
- `bug`: Something isn't working as expected
- `enhancement`: New feature or request
- `documentation`: Documentation improvements
- `good first issue`: Good for newcomers
- `help wanted`: Extra attention is needed
- `question`: Further information is requested

## Release Process

Midaz follows semantic versioning (MAJOR.MINOR.PATCH):
- **MAJOR**: Incompatible API changes
- **MINOR**: Backward-compatible functionality additions
- **PATCH**: Backward-compatible bug fixes

The release process is managed by the core team and involves:
1. Version bump according to SemVer rules
2. Changelog generation from conventional commits
3. Tag creation and release publishing
4. Binary and container image building and publishing

## Community Guidelines

### Code of Conduct

All contributors are expected to adhere to our [Code of Conduct](../../CODE_OF_CONDUCT.md), which promotes a respectful and inclusive community.

### Communication Channels

- **GitHub Issues**: For bug reports, feature requests, and discussions
- **Pull Requests**: For code review and contribution discussions
- **Discord**: For real-time community discussions and support

### Recognition

All contributors are acknowledged in our release notes and on our contributors page.

---

By following these guidelines, you help ensure Midaz is a welcoming, efficient, and valuable project for everyone. Thank you for your contributions and for being a part of our community!

For more general information about contributing, please refer to the [CONTRIBUTING.md](../../CONTRIBUTING.md) file in the project root.