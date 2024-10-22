AUTH_DIR := ./components/auth
INFRA_DIR := ./components/infra
LEDGER_DIR := ./components/ledger
TRANSACTION_DIR := ./components/transaction

.PHONY: auth ledger transaction

help:
	@echo "Management commands"
	@echo ""
	@echo "Usage:"
	@echo "  ## Root Commands"
	@echo "    make build                               Build all project services."
	@echo "    make test                                Run tests on all projects."
	@echo "    make clean                               Clean the directory tree of produced artifacts."
	@echo "    make lint                                Run static code analysis (lint)."
	@echo "    make format                              Run code formatter."
	@echo "    make checkEnvs                           Check if github hooks are installed and secret env on files are not exposed."
	@echo "    make auth                                Run a command inside the auth app in the components directory to see available commands."
	@echo "    make infra                               Run a command inside the infra app in the components directory to see available commands."
	@echo "    make ledger                              Run a command inside the ledger app in the components directory to see available commands."
	@echo "    make transaction                         Run a command inside the transaction app in the components directory to see available commands."
	@echo "    make all-services                        Run a command to all services passing any individual container command."
	@echo ""
	@echo "  ## Utility Commands"
	@echo "    make setup-git-hooks                     Command to setup git hooks."
	@echo ""

build:
	./make.sh "build"

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


set_env:
	cp -r $(AUTH_DIR)/.env.example $(AUTH_DIR)/.env
	cp -r $(INFRA_DIR)/.env.example $(INFRA_DIR)/.env
	cp -r $(LEDGER_DIR)/.env.example $(LEDGER_DIR)/.env
	cp -r $(TRANSACTION_DIR)/.env.example $(TRANSACTION_DIR)/.env

auth:
	$(MAKE) -C $(AUTH_DIR) $(COMMAND)

infra:
	$(MAKE) -C $(INFRA_DIR) $(COMMAND)

ledger:
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND)

transaction:
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

all-services:
	$(MAKE) -C $(AUTH_DIR) $(COMMAND) && \
	$(MAKE) -C $(INFRA_DIR) $(COMMAND) && \
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND) && \
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)
