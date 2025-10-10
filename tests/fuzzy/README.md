# Fuzz Testing

Fuzz tests are a form of automated testing that involves providing invalid,
unexpected, or random data as inputs to a computer program. The goal is to
discover software bugs, security vulnerabilities, and other issues that might
not be found with traditional testing methods.

The fuzz tests in this directory target API inputs with random and edge-case
values to validate robust parsing and error handling.

## How to Run

Run the fuzzers locally with the following command:
`go test -v ./tests/fuzzy -fuzz=Fuzz -run=^$`
