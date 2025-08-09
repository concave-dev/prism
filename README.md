# Prism

> [!CAUTION]
> The cluster is not yet stable. Use at your own risk.

Distributed runtime platform for AI agents and workflows.

## Design Philosophy

**Control Plane (CP):** Prioritizes Consistency + Partition tolerance. Raft consensus ensures no split-brain decisions but sacrifices availability during network partitions.

**Data Plane (AP):** Prioritizes Availability + Partition tolerance. Running workloads continue execution even when control plane is unreachable.

## Build

```bash
go build -o bin/prismd cmd/prismd/main.go
go build -o bin/prismctl cmd/prismctl/main.go
```

## Usage

Start the daemon:
```bash
# For local development
./bin/prismd --serf=0.0.0.0:4200 --name=first-node

# For production with remote API access
./bin/prismd --serf=0.0.0.0:4200 --api=0.0.0.0:8008 --name=first-node

# Join second node 
./bin/prismd --join=192.168.1.100:4200 --serf=0.0.0.0:4201 --name=second-node
```

Use the CLI:
```bash
# Show cluster overview
./bin/prismctl info

# List all nodes with status
./bin/prismctl node ls

# Show resource usage across cluster  
./bin/prismctl node top

# Get detailed node information
./bin/prismctl node info <node-name-or-id>

# Connect to remote cluster
./bin/prismctl --api=127.0.0.1:8008 info
```


## Current Status

### Implementation Status

**Core Infrastructure:**
- [x] Cluster membership via Serf gossip protocol
- [x] Raft consensus for leader election and state consistency  
- [x] HTTP REST API with endpoints for cluster/node information
- [x] Resource monitoring and collection across nodes
- [x] Graceful daemon startup/shutdown with auto-recovery
- [x] Multi-node clusters with automatic peer discovery
- [x] CLI tool with cluster overview and node management
- [x] JSON/table output formats for scripting and debugging
- [x] Remote cluster access via API endpoints

**Workload Management & AI Platform:**
- [ ] VM/Container Scheduling (core workload orchestration)
- [ ] Firecracker Integration (micro-VM management for sandboxed execution)
- [ ] Agent Mesh (service discovery and communication for AI agents)
- [ ] MCP Protocol Support (tool and workflow execution primitives)
- [ ] Workflow Engine (multi-step AI agent coordination)

**Infrastructure Services:**
- [ ] External Memory (RAG) (lightweight RAG layer for context injection)
- [ ] Observability (built-in metrics, tracing, and monitoring)
- [ ] Artifact Registry (storage and versioning for code, models, dependencies)
- [ ] LLM Gateway (centralized API gateway with routing, rate limiting)
- [ ] Secrets Vault (secure storage and distribution of credentials)
- [ ] Global Configuration (dynamic cluster-wide configuration management)
- [ ] Authentication & Authorization (fine-grained access control)

**Foundation (In Progress):**
- [ ] gRPC Server (inter-node communication layer - foundation in place)
- [ ] Enhanced Raft Usage (expanding beyond membership to workload state)
- [ ] Security Layer (TLS, authentication, authorization - TODO items present)

## Known Issues

- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts. Use odd-numbered clusters (3+) for proper quorum.
- **Network Connectivity:** Nodes behind NAT or firewalls may fail to join. Ensure gossip ports are accessible between all nodes.
- **Resource Query Timeouts:** Resource collection via Serf queries may timeout in slow networks or large clusters (>50 nodes).
- **Limited Production Readiness:** Missing TLS, auth, and robust error recovery needed for production use.