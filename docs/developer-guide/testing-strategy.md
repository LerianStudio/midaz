# Testing Strategy

**Navigation:** [Home](../) > [Developer Guide](../) > Testing Strategy

This document outlines the testing strategy used in the Midaz platform. It covers the different types of tests, testing patterns, and best practices for ensuring code quality and reliability.

## Table of Contents

- [Overview](#overview)
- [Testing Hierarchy](#testing-hierarchy)
- [Unit Testing](#unit-testing)
- [Integration Testing](#integration-testing)
- [Acceptance/System Testing](#acceptancesystem-testing)
- [Testing Tools](#testing-tools)
- [Mocking Strategy](#mocking-strategy)
- [Test Coverage](#test-coverage)
- [Best Practices](#best-practices)
- [Continuous Integration](#continuous-integration)

## Overview

Midaz employs a comprehensive testing strategy that follows industry best practices for financial systems. The primary goals of our testing approach are:

- **Reliability**: Ensure the system behaves correctly under all conditions
- **Maintainability**: Tests should be easy to understand and maintain
- **Coverage**: Critical code paths must be thoroughly tested
- **Isolation**: Tests should be independent and free from side effects
- **Performance**: Tests should run quickly to provide fast feedback
- **Documentation**: Tests serve as documentation for expected behavior

The testing approach is particularly rigorous for financial operations, where correctness and data integrity are paramount. Special attention is given to testing all edge cases in financial transaction processing.

## Testing Hierarchy

Midaz follows a testing pyramid approach with different types of tests:

1. **Unit Tests** (~70% of tests)
   - Test individual functions and methods in isolation
   - Focus on business logic, domain rules, and edge cases
   - Run extremely fast and provide immediate feedback

2. **Integration Tests** (~20% of tests)
   - Test the integration between multiple components
   - Verify correct interaction between services and external systems
   - Test database interactions, message queues, and API endpoints

3. **Acceptance/System Tests** (~10% of tests)
   - Test complete end-to-end scenarios
   - Simulate user workflows and business processes
   - Verify the system as a whole meets requirements

This hierarchy ensures comprehensive test coverage while prioritizing fast feedback loops with unit tests.

## Unit Testing

Unit tests are the foundation of the testing strategy and focus on testing small, isolated components of the system.

### Key Principles

- **Isolation**: Dependencies are mocked or stubbed to isolate the unit being tested
- **Structure**: Tests follow the Arrange-Act-Assert (AAA) pattern
- **Coverage**: Strive for high code coverage, especially for business logic
- **Behavior**: Focus on testing behavior, not implementation details

### Example Unit Test

```go
// Example unit test from components/onboarding/internal/services/command/create-account_test.go
func TestCreateAccountScenarios(t *testing.T) {
    // Setup test with mocks
    setupTest := func(ctrl *gomock.Controller) (*UseCase, *asset.MockRepository, *portfolio.MockRepository, *account.MockRepository, *rabbitmq.MockProducerRepository, *mongodb.MockRepository) {
        mockAssetRepo := asset.NewMockRepository(ctrl)
        mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
        mockAccountRepo := account.NewMockRepository(ctrl)
        mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
        mockMetadataRepo := mongodb.NewMockRepository(ctrl)

        uc := &UseCase{
            AssetRepo:     mockAssetRepo,
            PortfolioRepo: mockPortfolioRepo,
            AccountRepo:   mockAccountRepo,
            RabbitMQRepo:  mockRabbitMQ,
            MetadataRepo:  mockMetadataRepo,
        }

        return uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo
    }

    // Test cases
    tests := []struct {
        name         string
        input        *mmodel.CreateAccountInput
        mockSetup    func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository)
        expectedErr  string
        expectedName string
        expectError  bool
    }{
        {
            name: "success with all fields",
            input: &mmodel.CreateAccountInput{
                Name:      "Test Account",
                Type:      "deposit",
                AssetCode: "USD",
            },
            mockSetup: func(mockAssetRepo *asset.MockRepository, mockPortfolioRepo *portfolio.MockRepository, mockAccountRepo *account.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository, mockMetadataRepo *mongodb.MockRepository) {
                mockAssetRepo.EXPECT().
                    FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
                    Return(true, nil).AnyTimes()

                mockAccountRepo.EXPECT().
                    Create(gomock.Any(), gomock.Any()).
                    DoAndReturn(func(_ context.Context, account *mmodel.Account) (*mmodel.Account, error) {
                        account.ID = uuid.New().String()
                        return account, nil
                    }).AnyTimes()
            },
            expectedName: "Test Account",
            expectError:  false,
        },
        // Additional test cases...
    }

    // Run tests
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Arrange
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()
            uc, mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo := setupTest(ctrl)
            tt.mockSetup(mockAssetRepo, mockPortfolioRepo, mockAccountRepo, mockRabbitMQ, mockMetadataRepo)

            // Act
            account, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input)

            // Assert
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedErr)
                assert.Nil(t, account)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, account)
                assert.Equal(t, tt.expectedName, account.Name)
            }
        })
    }
}
```

### Table-Driven Tests

The Midaz codebase favors table-driven tests to efficiently test multiple scenarios within the same test function. This approach:

- Reduces code duplication
- Makes it easy to add new test cases
- Provides clear documentation of expected behavior for different inputs
- Makes test maintenance more manageable

## Integration Testing

Integration tests verify the interaction between different components of the system.

### Key Areas Covered

- **Repository Layer**: Tests database interactions with actual database instances
- **API Endpoints**: Tests RESTful API endpoints with actual HTTP requests
- **Message Processing**: Tests message queue producers and consumers
- **Inter-Service Communication**: Tests communication between services

### Integration Test Example

```go
//go:build integration
// +build integration

package integration

import (
    "fmt"
    "os/exec"
    "testing"
    "gotest.tools/golden"
)

func TestMDZ(t *testing.T) {
    var stdout string

    // Test login
    stdout, _ = cmdRun(t, exec.Command("mdz", "login",
        "--username", "user_john",
        "--password", "Lerian@123",
    ))
    golden.AssertBytes(t, []byte(stdout), "out_login_flags.golden")

    // Test organization creation
    stdout, _ = cmdRun(t, exec.Command("mdz", "organization", "create",
        "--legal-name", "Soul LLCT",
        "--doing-business-as", "The ledger.io",
        "--legal-document", "48784548000104",
        "--code", "ACTIVE",
        "--description", "Test Ledger",
        "--line1", "Av Santso",
        "--line2", "VJ 222",
        "--zip-code", "04696040",
        "--city", "West",
        "--state", "VJ",
        "--country", "MG",
        "--metadata", `{"chave1": "valor1", "chave2": 2,  "chave3": true}`,
    ))

    // Additional tests for other operations...
}
```

### Golden Files

The project uses golden files to maintain reference output for integration tests. This approach:

- Provides a stable reference for expected outputs
- Makes it easy to update expected outputs when necessary
- Clearly shows the difference between expected and actual output

## Acceptance/System Testing

Acceptance tests verify that the system as a whole meets business requirements. These tests:

- Simulate complete user workflows
- Test end-to-end business processes
- Verify the system's interaction with external dependencies
- Focus on business requirements rather than technical implementation

The Midaz acceptance testing approach includes:

- **API Contract Tests**: Verify API endpoints conform to the OpenAPI specification
- **End-to-End Workflow Tests**: Test complete financial workflows
- **Performance Tests**: Verify performance under load for critical operations

## Testing Tools

Midaz employs several testing tools and libraries:

### Core Testing Tools

- **testing**: Go's standard testing package
- **testify/assert**: Enhanced assertion utilities
- **gomock**: Mocking framework for interfaces
- **golden**: Comparison of test output with reference files

### Command-Line Tools

- **go test**: Run tests via command-line
- **make test**: Midaz Makefile target for running tests
- **make cover**: Generate test coverage reports

### Example Makefile Test Target

```makefile
.PHONY: test
test:
    $(call title1,"Running tests on all components")
    $(call check_command,go,"Install Go from https://golang.org/doc/install")
    @echo "$(CYAN)Starting tests at $$(date)$(NC)"
    @start_time=$$(date +%s); \
    test_output=$$(go test -v ./... 2>&1); \
    exit_code=$$?; \
    end_time=$$(date +%s); \
    duration=$$((end_time - start_time)); \
    echo "$$test_output"; \
    echo ""; \
    echo "$(BOLD)$(BLUE)Test Summary:$(NC)"; \
    echo "$(CYAN)----------------------------------------$(NC)"; \
    passed=$$(echo "$$test_output" | grep -c "PASS"); \
    failed=$$(echo "$$test_output" | grep -c "FAIL"); \
    skipped=$$(echo "$$test_output" | grep -c "\[no test"); \
    total=$$((passed + failed)); \
    echo "$(GREEN)✓ Passed:  $$passed tests$(NC)"; \
    if [ $$failed -gt 0 ]; then \
        echo "$(RED)✗ Failed:  $$failed tests$(NC)"; \
    else \
        echo "$(GREEN)✓ Failed:  $$failed tests$(NC)"; \
    fi; \
    echo "$(YELLOW)⚠ Skipped: $$skipped packages [no test files]$(NC)"; \
    echo "$(BLUE)⏱ Duration: $$(printf '%dm:%02ds' $$((duration / 60)) $$((duration % 60)))$(NC)"; \
    echo "$(CYAN)----------------------------------------$(NC)"; \
    if [ $$failed -eq 0 ]; then \
        echo "$(GREEN)$(BOLD)All tests passed successfully!$(NC)"; \
    else \
        echo "$(RED)$(BOLD)Some tests failed. Please check the output above for details.$(NC)"; \
    fi; \
    exit $$exit_code
```

## Mocking Strategy

Midaz uses a consistent approach to mocking dependencies in tests:

### Mock Generation

- **gomock**: Used to generate mock implementations of interfaces
- **make regenerate-mocks**: Makefile target to regenerate all mocks
- **make cleanup-regenerate-mocks**: Clean and regenerate all mocks

### Mock Conventions

- Mock files are named with a `_mock.go` suffix
- Mocks are placed in the same package as the interface they implement
- Mocks are automatically generated from interface definitions

### Dependency Injection

Midaz employs dependency injection to facilitate testing:

- Services accept dependencies via constructor parameters
- Interfaces are defined for all external dependencies
- Production and test code use the same interfaces

## Test Coverage

Test coverage is an important metric but is not the sole criterion for quality:

### Coverage Goals

- **Business Logic**: Target 80%+ coverage
- **Critical Paths**: Target 90%+ coverage
- **Infrastructure Code**: Target 60%+ coverage

### Coverage Commands

- **make cover**: Generate coverage report and HTML visualization
- **make check-tests**: Verify test coverage meets thresholds

### Coverage Exclusions

Some code is explicitly excluded from coverage calculations:

- Generated code (e.g., mocks, protobuf)
- Main packages and initialization code
- Pure data structures without behavior

## Best Practices

### Code Organization

- Test files are placed alongside the code they test with a `_test.go` suffix
- Helper functions are kept in the same test file or in dedicated test utility packages
- Shared test fixtures are placed in `testdata` directories

### Test Naming

- Test functions are named with a `Test` prefix followed by the function or scenario being tested
- Table-driven test cases have descriptive names explaining the scenario

### Assertions

- Use `testify/assert` for consistent assertions
- Prefer specific assertions (e.g., `assert.Equal`) over generic ones
- Include meaningful error messages in assertions

### Common Test Patterns

- **Repository Tests**: Test database interactions
- **Service Tests**: Test business logic with mocked dependencies
- **Handler Tests**: Test API handlers with mocked services
- **End-to-End Tests**: Test complete workflows

## Continuous Integration

Midaz integrates testing into the CI/CD pipeline:

### CI Pipeline Steps

1. **Lint**: Check code style and identify potential issues
2. **Unit Tests**: Run all unit tests
3. **Integration Tests**: Run integration tests in isolated environments
4. **Coverage**: Generate and check coverage reports
5. **Build**: Ensure the code can be built successfully

### Pre-Commit Hooks

Local development uses Git hooks to enforce quality:

- **pre-commit**: Run linters and formatters
- **pre-push**: Run unit tests

These hooks can be installed with:

```bash
make setup-git-hooks
```

### Test Automation

Tests are automatically triggered on:

- Pull request creation or updates
- Merges to main branches
- Scheduled runs for stability verification