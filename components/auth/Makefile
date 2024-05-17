AUTH_DIR := ./components/auth

.PHONY: auth

build:
	./make.sh "build"

help:
	@echo "Management commands"
	@echo ""
	@echo "Usage:"
	@echo "  ## Root Commands"
	@echo "    make build           Build all project services."
	@echo "    make test            Run tests on all projects."
	@echo "    make clean           Clean the directory tree of produced artifacts."
	@echo "    make lint            Run static code analysis (lint)."
	@echo "    make format          Run code formatter."
	@echo "    make checkEnvs       Check if github hooks are instaled and secret env on files are not exposed."
	@echo "    make gen             Generates all project code to connect its components using Wire."
	@echo ""
	@echo "  ## Utility Commands"
	@echo "    make setup-git-hooks       Setup git hooks."
	@echo ""

test:
	go test -v ./... ./...

cover:
	go test -cover ./...

clean:
	./make.sh "clean"

lint:
	./make.sh "lint"

format:
	./make.sh "format"

check-logs:
	./make.sh "checkLogs"

check-tests:
	./make.sh "checkTests"

setup-git-hooks:
	./make.sh "setupGitHooks"

goreleaser:
	goreleaser release --snapshot --skip-publish --rm-dist

tidy:
	go mod tidy

sec:
	gosec ./...

ledger:
	$(MAKE) -C $(AUTH_DIR) restart

gen:
	$(MAKE) -C $(AUTH_DIR) gen