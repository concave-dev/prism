.PHONY: build prismd prismctl clean clean-bin stop delete-dir generate-grpc test test-coverage run run-cluster bootstrap-expect

# Environment Variables:
# NODES: Number of cluster nodes to start (default: 3)
#
# Port Management:
# - Makefile automatically sets MAX_PORTS = NODES + 50
# - For 150 nodes: MAX_PORTS=200 (provides 50-port buffer)

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
	@echo "Killing by port ranges (Serf 4200-4499 TCP+UDP, Raft 6969-7268 TCP, gRPC 7117-7416 TCP, API 8008-8307 TCP)"
	@start=4200; end=4499; \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true; \
		pids=$$(lsof -ti udp:$$start-$$end 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=6969; end=7268; \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=7117; end=7416; \
		pids=$$(lsof -ti tcp:$$start-$$end -sTCP:LISTEN 2>/dev/null); [ -n "$$pids" ] && kill -9 $$pids 2>/dev/null || true
	@start=8008; end=8307; \
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
	@$(MAKE) run-cluster NODES=3

# Dynamic cluster creation with configurable node count  
# Usage: make run-cluster NODES=N (default: NODES=3)
run-cluster: build stop
	@NODES=$${NODES:-3}; \
	MAX_PORTS=$$((NODES + 50)); \
	if [ $$NODES -lt 1 ]; then \
		echo "Error: NODES must be at least 1"; \
		exit 1; \
	fi; \
	echo "=== Starting Prism cluster with $$NODES nodes (MAX_PORTS: $$MAX_PORTS) ==="; \
	if [ $$NODES -eq 1 ]; then \
		echo "Starting single bootstrap node..."; \
		MAX_PORTS=$$MAX_PORTS $(PRISMD) --bootstrap & \
		echo "Single node cluster started! Use 'make stop' to stop."; \
	else \
		echo "Using bootstrap-expect mode for $$NODES nodes..."; \
		echo "Starting first 3 nodes (bootstrap cluster)..."; \
		MAX_PORTS=$$MAX_PORTS $(PRISMD) --serf=0.0.0.0:4200 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 & \
		sleep 5; \
		MAX_PORTS=$$MAX_PORTS $(PRISMD) --serf=0.0.0.0:4201 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 & \
		sleep 5; \
		MAX_PORTS=$$MAX_PORTS $(PRISMD) --serf=0.0.0.0:4202 --bootstrap-expect=3 --join=0.0.0.0:4200,0.0.0.0:4201,0.0.0.0:4202 & \
		echo "Waiting 15 seconds for initial 3-node cluster to form and elect leader..."; \
		sleep 15; \
		if [ $$NODES -gt 3 ]; then \
			echo "Adding remaining $$(($$NODES-3)) nodes to established cluster..."; \
			for i in $$(seq 4 $$NODES); do \
				echo "Starting node $$i/$$NODES (joining established cluster)..."; \
				MAX_PORTS=$$MAX_PORTS $(PRISMD) --join=0.0.0.0:4200 & \
				sleep 4; \
			done; \
		fi; \
		echo "Cluster will form once all $$NODES nodes are discovered. Use 'make stop' to stop all nodes."; \
	fi

bootstrap-expect: build stop
	@echo "=== Starting Prism cluster with bootstrap-expect (3 nodes) ==="
	@echo "Starting first node (bootstrap seed)..."
	@$(PRISMD) --bootstrap-expect=3 &
	@sleep 5
	@echo "Starting second node (joining cluster)..."
	@$(PRISMD) --bootstrap-expect=3 --join=0.0.0.0:4200 &
	@sleep 5
	@echo "Starting third node (joining cluster)..."
	@$(PRISMD) --bootstrap-expect=3 --join=0.0.0.0:4200 &
	@echo "Cluster will form once all 3 nodes are discovered. Use 'make stop' to stop all nodes."

# Pattern rule for any number of nodes: make run-N
run-%:
	@$(MAKE) run-cluster NODES=$*


