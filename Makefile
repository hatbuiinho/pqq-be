SHELL := /bin/bash

.PHONY: sqlc dev test fmt

sqlc:
	./scripts/sqlc-generate.sh

dev:
	air

test:
	GOCACHE=$(CURDIR)/.gocache go test ./...

fmt:
	gofmt -w ./cmd ./internal
