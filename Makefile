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
			--authrpc-backend http://127.0.0.1:8651 \
			--authrpc-enabled \
			--authrpc-listen-address 127.0.0.1:18651 \
			--authrpc-log-requests \
			--authrpc-log-responses \
			--authrpc-max-request-size 150 \
			--authrpc-max-response-size 1150 \
			--rpc-backend http://127.0.0.1:8645 \
			--rpc-enabled \
			--rpc-listen-address 127.0.0.1:18645 \
			--rpc-log-requests \
			--rpc-log-responses
			--rpc-max-request-size 150 \
			--rpc-max-response-size 1150 \
