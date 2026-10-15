NAME=dac$(shell if [ "$(shell go env GOOS)" = "windows" ]; then echo .exe; fi)
BUILD_DIR ?= bin
BUILD_SRC=.
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
TELEMETRY_KEY ?=
GO_LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.telemetryKey=$(TELEMETRY_KEY)

NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m

# Suppress CGO linker warnings on macOS (not needed on Linux/Windows)
ifeq ($(shell go env GOOS),darwin)
export CGO_LDFLAGS=-Wl,-w
export LDFLAGS=-Wl,-w
endif

GO_PACKAGES ?= ./cmd/... ./pkg/... ./schemas/...
GOFMT_FILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: all clean test build frontend deps format format-go format-frontend dev dev-frontend dev-backend

all: clean deps test build

deps:
	@printf "$(OK_COLOR)==> Installing dependencies$(NO_COLOR)\n"
	@go mod tidy
	@cd frontend && npm ci --legacy-peer-deps

# Build the frontend assets used for Go embedding
frontend:
	@echo "$(OK_COLOR)==> Building the frontend...$(NO_COLOR)"
	@cd frontend && npm run build

# Build the Go binary (requires frontend assets to be built first)
build: frontend
	@echo "$(OK_COLOR)==> Building the application...$(NO_COLOR)"
	@go build -v -ldflags="$(GO_LDFLAGS)" -o "$(BUILD_DIR)/$(NAME)" "$(BUILD_SRC)"

clean:
	@rm -rf ./bin ./frontend/dist

test: test-unit

test-unit:
	@echo "$(OK_COLOR)==> Running the unit tests$(NO_COLOR)"
	@go test -race -cover -timeout 5m $(GO_PACKAGES)

format: format-go format-frontend
	@printf "$(OK_COLOR)==> Static checks passed$(NO_COLOR)\n"

format-go:
	@printf "$(OK_COLOR)>> [gofmt] checking$(NO_COLOR)\n"
	@unformatted="$$(gofmt -s -l $(GOFMT_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		printf "$(ERROR_COLOR)gofmt needed on:$(NO_COLOR)\n$$unformatted\n"; \
		printf "Run: gofmt -s -w <files>\n"; \
		exit 1; \
	fi
	@printf "$(OK_COLOR)>> [go vet] running$(NO_COLOR)\n"
	@go vet $(GO_PACKAGES)

format-frontend:
	@printf "$(OK_COLOR)>> [frontend typecheck] running$(NO_COLOR)\n"
	@cd frontend && npm run typecheck
	@printf "$(OK_COLOR)>> [frontend lint] running$(NO_COLOR)\n"
	@cd frontend && npm run lint

# Run both frontend and backend with live reload
dev:
	@trap 'kill 0' EXIT; \
	$(MAKE) dev-backend & \
	$(MAKE) dev-frontend & \
	wait

# Run frontend dev server (with API proxy to Go backend on :8321)
dev-frontend:
	@cd frontend && npm run dev

# Run Go backend with live reload via air
dev-backend:
	@air
