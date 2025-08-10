# Prism

> [!CAUTION]
> The cluster is not yet stable. Use at your own risk.

Distributed runtime platform for AI agents and workflows.

## Philosophy: Accelerating AI Development Cycles

The AI agent landscape is evolving rapidly, but current tooling creates slow feedback loops that hinder adaptation. As noted in [Adapt Fast In The AI Era](https://matmul.net/$/adapt-fast.html), the ecosystem is fragmented with countless frameworks, each requiring different deployment strategies and lacking standardized inter-agent communication.

Meanwhile, [the real bottleneck in software development](https://ordep.dev/posts/writing-code-was-never-the-bottleneck) isn't writing code—it's understanding, testing, and trusting it. LLMs can generate code faster than ever, but the human overhead of coordination, review, and integration remains.

**Prism addresses both problems:**

- **Unified Runtime**: Deploy agents written in any language/framework through standardized primitives
- **Fast Iteration**: Sandboxed execution with instant feedback eliminates deployment friction  
- **Built-in Observability**: Understanding and trusting agent behavior through comprehensive monitoring
- **Agent Mesh**: Standardized inter-agent communication reduces integration complexity

The goal is simple: **compress the time from idea to deployed, observable, trustworthy agent from weeks to minutes**. In a rapidly evolving field, survival depends on adaptation speed.

This aligns with [Concave's mission](https://concave.dev/) to build unified infrastructure for the emerging "Internet of Agents"—where current infrastructure designed for predictable workflows fails to support autonomous agents that need native primitives, security, and observability.

## Technical Design Philosophy

**Space-Time Fabric Architecture:** Prism operates as a distributed space-time fabric where agents, tools, and workflows exist as first-class entities that can be deployed, scheduled, and coordinated across the cluster. The fabric provides the foundational layer for all agentic operations while maintaining modularity.

**Modular Infrastructure Services:** Core services like memory (RAG), observability, secrets, and configuration are designed as pluggable modules that attach to the fabric. This allows incremental adoption and customization while maintaining system coherence.

**Dual-Layer Design (CAP Theorem):**

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