#!/bin/bash

IGNORED=$(cat ./scripts/coverage_ignore.txt | xargs -I{} echo '-not -path ./{}/*' | xargs)
PACKAGES=$(go list ./pkg/... ./components/... | grep -v -f ./scripts/coverage_ignore.txt)

echo $PACKAGES
go test -cover $PACKAGES -coverprofile=coverage.out