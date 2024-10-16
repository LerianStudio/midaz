GO := $(shell which go)
NAME := mdz

ifeq (, $(GO))
$(error "No go binary found in your system, please install go version go1.23.2 before continuing")
endif

.PHONY: get-lint-deps
get-lint-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: lint
lint: get-lint-deps
	golangci-lint run ./... --verbose

.PHONY: get-govulncheck-deps
get-govulncheck-deps:
	go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: govulncheck
govulncheck: get-govulncheck-deps
	govulncheck ./...

.PHONY: gosec
gosec: get-gosec-deps
	 gosec ./...

.PHONY: get-gosec-deps
get-gosec-deps:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY : build
build:
	go version
	go build -o ./bin/$(NAME) ./main.go
