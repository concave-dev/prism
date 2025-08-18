.PHONY: build prismd prismctl clean stop-prismd delete-dir generate-grpc test

BIN_DIR := bin
PRISMD := $(BIN_DIR)/prismd
PRISMCTL := $(BIN_DIR)/prismctl
PROTO := internal/grpc/proto/node_service.proto
GO := go

build: $(PRISMD) $(PRISMCTL)

prismd: $(PRISMD)

prismctl: $(PRISMCTL)

$(PRISMD):
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(PRISMD) ./cmd/prismd

$(PRISMCTL):
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(PRISMCTL) ./cmd/prismctl

stop-prismd:
	@echo "=== Stopping prismd processes ==="
	-@pkill -f prismd 2>/dev/null || true
	@echo "Killing by port ranges (Serf 4200-4219 TCP+UDP, Raft 6969-6988 TCP, gRPC 7117-7136 TCP, API 8008-8027 TCP)"
	@start=4200; end=$$((start+20-1)); \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true; \
		pids=$$(lsof -ti udp:$$start-$$end 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=6969; end=$$((start+20-1)); \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=7117; end=$$((start+20-1)); \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=8008; end=$$((start+20-1)); \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true

delete-dir:
	@echo "=== Deleting bin and data directories ==="
	@rm -rf $(BIN_DIR) data

clean: stop-prismd delete-dir

generate-grpc:
	@echo "=== Generating gRPC code ==="
	@export PATH="$$PATH:$$($(GO) env GOPATH)/bin"; \
		if ! command -v protoc >/dev/null 2>&1; then \
			echo "protoc not found. Install with: brew install protobuf"; exit 1; \
		fi; \
		if ! command -v protoc-gen-go >/dev/null 2>&1; then \
			$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
		fi; \
		if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
			$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest; \
		fi; \
		protoc \
			--go_out=. \
			--go-grpc_out=. \
			--go_opt=paths=source_relative \
			--go-grpc_opt=paths=source_relative \
			$(PROTO)

test:
	@echo "=== Running tests without cache ==="
	$(GO) test -count=1 ./...


