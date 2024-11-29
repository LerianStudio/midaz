GO := $(shell which go)
NAME := mdz

ifeq (, $(GO))
$(error "No go binary found in your system, please install go version go1.23.2 before continuing")
endif

ifneq (,$(wildcard .env))
    include .env
endif


LDFLAGS=-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientID=$(CLIENT_ID)'\
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientSecret=$(CLIENT_SECRET)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPIAuth=$(URL_API_AUTH)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPILedger=$(URL_API_LEDGER)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.Version=$(VERSION)'

.PHONY: get-lint-deps
get-lint-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: lint
lint: get-lint-deps
	golangci-lint run --fix ./... --verbose

.PHONY: get-govulncheck-deps
get-govulncheck-deps:
	go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: govulncheck
govulncheck: get-govulncheck-deps
	govulncheck ./...

.PHONY: get-gosec-deps
get-gosec-deps:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: gosec
gosec: get-gosec-deps
	 gosec ./...

.PHONY: get-perfsprint-deps
get-perfsprint-deps:
	go get github.com/catenacyber/perfsprint@latest

.PHONY : perfsprint
perfsprint: get-perfsprint-deps
	perfsprint ./...

.PHONY: test
test: 
	 go test ./...

.PHONY: test_integration
test_integration:
	go test -v -tags=integration ./test/integration/...

.PHONY: build
build:
	go version
	go build -ldflags "$(LDFLAGS)" -o ./bin/$(NAME) ./main.go

.PHONY: install-local
install-local: build
	sudo cp -r bin/mdz /usr/local/bin
