VERSION := $(shell git describe --tags --always --dirty="-dev" --match "v*.*.*" || echo "development" )
VERSION := $(VERSION:v%=%)

.PHONY: build
build:
	@CGO_ENABLED=0 go build \
			-ldflags "-X main.version=${VERSION}" \
			-o ./bin/bproxy \
		github.com/flashbots/bproxy/cmd

.PHONY: snapshot
snapshot:
	@goreleaser release --snapshot --clean

.PHONY: help
help:
	@go run github.com/flashbots/bproxy/cmd serve --help

.PHONY: serve
serve:
	@go run github.com/flashbots/bproxy/cmd \
			--log-level info \
		serve \
			--authrpc-backend http://127.0.0.1:18651 \
			--authrpc-listen-address 127.0.0.1:8651 \
			--authrpc-peers http://127.0.0.1:28651 \
			--rpc-backend http://127.0.0.1:18645 \
			--rpc-listen-address 127.0.0.1:8645 \
			--rpc-peers http://127.0.0.1:28645
