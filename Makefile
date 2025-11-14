test: test-memiavl test-versiondb test-store

test-store:
	@go test -v -mod=readonly ./...;

test-memiavl:
	@cd memiavl; go test -v -mod=readonly ./...;

test-versiondb:
	@cd versiondb; go test -tags=rocksdb -v -mod=readonly ./...;

.PHONY: test test-memiavl test-store test-versiondb
