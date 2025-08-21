.PHONY: build prismd prismctl clean clean-bin stop delete-dir generate-grpc test test-coverage run bootstrap-expect

BIN_DIR := bin
PRISMD := $(BIN_DIR)/prismd
PRISMCTL := $(BIN_DIR)/prismctl
PROTO := internal/grpc/proto/node_service.proto
GO := go
BOOTSTRAP := true

build: clean-bin $(PRISMD) $(PRISMCTL)

prismd: clean-bin $(PRISMD)

prismctl: clean-bin $(PRISMCTL)

clean-bin:
	@echo "=== Removing old binaries ==="
	@rm -rf $(BIN_DIR)

$(PRISMD):
	@mkdir -p $(BIN_DIR)
	@echo "=== Building prismd ==="
	$(GO) build -o $(PRISMD) ./cmd/prismd

$(PRISMCTL):
	@mkdir -p $(BIN_DIR)
	@echo "=== Building prismctl ==="
	$(GO) build -o $(PRISMCTL) ./cmd/prismctl

stop:
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
	@echo "=== Cleaning coverage files ==="
	@rm -f coverage.out coverage.html *.prof *.pprof

clean: stop delete-dir

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

test-coverage:
	@echo "=== Running tests with coverage analysis ==="
	$(GO) test -coverprofile=coverage.out ./...
	@echo "=== Coverage Report ==="
	$(GO) tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

run: build stop
	@echo "=== Starting Prism cluster (1 bootstrap + 2 joining nodes) ==="
	@echo "Starting bootstrap node..."
	@if [ "$(BOOTSTRAP)" = "true" ]; then \
		$(PRISMD) --bootstrap & \
	else \
		$(PRISMD) --bootstrap-expect=3 & \
	fi
	@sleep 3
	@echo "Starting joining node 1..."
	@if [ "$(BOOTSTRAP)" = "true" ]; then \
		$(PRISMD) --join=0.0.0.0:4200 & \
	else \
		$(PRISMD) --bootstrap-expect=3 --join=0.0.0.0:4200 & \
	fi
	@sleep 2
	@echo "Starting joining node 2..."
	@if [ "$(BOOTSTRAP)" = "true" ]; then \
		$(PRISMD) --join=0.0.0.0:4200 & \
	else \
		$(PRISMD) --bootstrap-expect=3 --join=0.0.0.0:4200 & \
	fi
	@echo "Cluster started! Use 'make stop' to stop all nodes."

bootstrap-expect: build stop
	@echo "=== Starting Prism cluster with bootstrap-expect (3 nodes) ==="
	@echo "Starting node 1 with bootstrap-expect=3..."
	@$(PRISMD) --serf=0.0.0.0:4200 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 &
	@sleep 2
	@echo "Starting node 2 with bootstrap-expect=3..."
	@$(PRISMD) --serf=0.0.0.0:4201 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 &
	@sleep 2
	@echo "Starting node 3 with bootstrap-expect=3..."
	@$(PRISMD) --serf=0.0.0.0:4202 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 &
	@echo "Cluster will form once all 3 nodes are discovered. Use 'make stop' to stop all nodes."


