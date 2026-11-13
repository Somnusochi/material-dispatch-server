SHELL := /bin/sh
# 项目默认镜像；环境变量或 make GOPROXY=... 可覆盖，不修改全局 Go 配置。
GOPROXY ?= https://goproxy.cn,direct
export GOPROXY
export GOPATH := $(CURDIR)/.cache/go
export GOMODCACHE := $(CURDIR)/.cache/go-mod
export GOCACHE := $(CURDIR)/.cache/go-build
export GOTOOLCHAIN := go$(shell awk '/^go / { print $$2; exit }' go.mod)
GO_FILES := $(shell find cmd internal migrations tests -name '*.go')

.PHONY: toolchain deps dev build fmt fmt-check lint typecheck test test-integration test-e2e-server check migrate-up migrate-status migrate-down

toolchain:
	go version
	@test "$$(go env GOVERSION)" = "$(GOTOOLCHAIN)"
dev:
	go run ./cmd/server
deps:
	go mod download
	go mod verify
build:
	go build -trimpath -o bin/server ./cmd/server
	go build -trimpath -o bin/migrate ./cmd/migrate
	go build -trimpath -o bin/admin ./cmd/admin
fmt:
	"$$(go env GOROOT)/bin/gofmt" -w $(GO_FILES)
fmt-check:
	@set -e; go_formatter="$$(go env GOROOT)/bin/gofmt"; \
	unformatted="$$("$$go_formatter" -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then printf '%s\n' "$$unformatted"; exit 1; fi
lint:
	go vet ./...
typecheck:
	go build ./...
test:
	go test -race -count=1 ./...
test-integration:
	@set -a; if [ -f .env ]; then . ./.env; fi; set +a; \
	test -n "$$TEST_DATABASE_URL" || { echo 'TEST_DATABASE_URL is required'; exit 1; }; \
	go test -p 1 -race -count=1 -v -tags=integration ./migrations ./internal/routing ./tests/integration
check:
	$(MAKE) toolchain fmt-check lint typecheck test test-integration build
migrate-up:
	go run ./cmd/migrate up
migrate-status:
	go run ./cmd/migrate status
migrate-down:
	go run ./cmd/migrate down

test-e2e-server:
	go run -tags=integration ./tests/e2eserver
