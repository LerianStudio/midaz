# Midaz Install Script Testing Framework

This directory contains a Docker-based testing framework for the Midaz install script. It allows you to test the installation process across multiple Linux distributions and architectures to ensure compatibility and reliability.

## Overview

The testing framework:

1. Creates Docker containers for various Linux distributions
2. Executes the install script in each environment
3. Generates a minimal set of demo data to verify functionality
4. Produces a comprehensive test report

## Supported Environments

Currently, the framework tests the install script on:

- Ubuntu 22.04
- Debian 11
- Fedora 37
- Alpine 3.16
- ARM64 (Ubuntu 22.04)

## Usage

To run the tests, make sure you have Docker and Docker Compose installed, then execute:

```bash
# Make the script executable
chmod +x run-tests.sh

# Run all tests sequentially
./run-tests.sh

# Run tests in parallel
./run-tests.sh --parallel

# Test a specific distribution only
./run-tests.sh --distro ubuntu

# Clean up before testing
./run-tests.sh --clean

# Generate a report from existing logs without running tests
./run-tests.sh --report-only
```

## Test Report

After running the tests, a report will be generated at `test-report.md`. This report includes:

- A summary table of test results for each distribution
- Pass/fail statistics
- Detailed error information for any failed tests

## Adding New Distributions

To add a new distribution to the test suite:

1. Create a new Dockerfile (e.g., `Dockerfile.centos`)
2. Add the new service to `docker-compose.yml`
3. The test runner will automatically include it in the test suite

## Troubleshooting

If you encounter issues:

- Check the logs in the `logs` directory for detailed error information
- Verify that Docker has sufficient resources allocated
- For ARM testing, ensure your Docker installation supports multi-architecture builds

## Notes

- The tests use a minimal demo data set to reduce resource usage
- Each test runs with the `--yes` flag to enable non-interactive mode
- The framework captures both stdout and stderr to the log files
