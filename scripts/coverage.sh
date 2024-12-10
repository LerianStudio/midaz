#!/bin/bash

IGNORED=$(cat ./scripts/coverage_ignore.txt | xargs -I{} echo '-not -path ./{}/*' | xargs)
PACKAGES=$(go list ./pkg/... | grep -v -f ./scripts/coverage_ignore.txt)

go test -cover $PACKAGES -coverprofile=coverage.out
