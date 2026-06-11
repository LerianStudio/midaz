# ------------------------------------------------------
# Protobuf / gRPC code generation
# ------------------------------------------------------
# Generates the reservation seam gRPC stubs from proto/ into pkg/proto/ using
# buf. buf and the protoc plugins run via `go run` / buf remote plugins at
# PINNED versions, so no global install is required and the gate stays
# deterministic. Keep BUF_VERSION here and the plugin pins in buf.gen.yaml in
# sync with go.mod (google.golang.org/protobuf, grpc/cmd/protoc-gen-go-grpc).
#
# Usage:
#   make proto         # regenerate stubs into pkg/proto/
#   make proto-lint    # buf lint the proto sources
#   make proto-check   # regenerate + fail if the committed stubs are stale (CI gate)
# ------------------------------------------------------

# Pinned buf version — single source of truth for the proto toolchain.
BUF_VERSION ?= v1.50.0
BUF := go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

.PHONY: proto proto-lint proto-check

# Regenerate the protobuf/gRPC stubs.
proto:
	$(call print_title,Generating protobuf/gRPC stubs)
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@$(BUF) lint
	@$(BUF) generate
	@echo "[ok] Protobuf stubs generated successfully"

# Lint the proto sources only (no codegen).
proto-lint:
	$(call print_title,Linting protobuf sources)
	@$(BUF) lint
	@echo "[ok] Protobuf lint passed"

# CI gate: regenerate and fail if the committed stubs drift from the proto.
# Uses `git status --porcelain` (not `git diff`) so the gate catches BOTH a
# modified tracked stub AND an untracked stub the author forgot to commit —
# `git diff` ignores untracked files, which would let a missing stub pass.
proto-check:
	$(call print_title,Verifying protobuf stubs are up to date)
	@$(BUF) lint
	@$(BUF) generate
	@if [ -n "$$(git status --porcelain -- pkg/proto)" ]; then \
		echo "[error] Generated protobuf stubs are stale or uncommitted. Run 'make proto' and commit the result."; \
		git status --porcelain -- pkg/proto; \
		exit 1; \
	fi
	@echo "[ok] Protobuf stubs are up to date"
