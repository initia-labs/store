DOCKER := $(shell which docker)

###############################################################################
###                           Tests 
###############################################################################

test: test-memiavl test-versiondb test-store

test-store:
	@go test -v -mod=readonly ./...;

test-memiavl:
	@cd memiavl; go test -v -mod=readonly ./...;

test-versiondb:
	@cd versiondb; go test -tags=rocksdb -v -mod=readonly ./...;

.PHONY: test test-memiavl test-store test-versiondb

###############################################################################
###                                Linting                                  ###
###############################################################################

lint:
	golangci-lint run --timeout=15m --tests=false

lint-fix:
	golangci-lint run --fix --timeout=15m --tests=false

.PHONY: lint lint-fix

###############################################################################
###                                Protobuf                                 ###
###############################################################################

protoVer=0.14.0
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage=$(DOCKER) run --rm -v $(CURDIR):/workspace  --workdir /workspace $(protoImageName)

proto-all: proto-format proto-lint proto-gen

proto-gen:
	@echo "Generating Protobuf files"
	@$(protoImage) sh ./scripts/protocgen.sh

proto-format:
	@$(protoImage) find ./ -name "*.proto" -exec buf format {} -w \;

proto-lint:
	@$(protoImage) buf lint --error-format=json ./proto

.PHONY: proto-all proto-gen proto-format proto-lint