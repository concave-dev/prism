# Prism

> [!CAUTION]
> The cluster is not yet stable. Use at your own risk. Test coverage: ~25%.

Distributed runtime platform for AI agents and workflows.

## Overview

Distributed runtime for AI agents with sandboxed execution, cluster management, and built-in observability.

The AI ecosystem is fragmented with slow deployment cycles. Current infrastructure wasn't designed for autonomous agents that need rapid iteration, inter-agent communication, and built-in observability. Prism aims to compress the time from idea to deployed agent from weeks to minutes.

This aligns with [Concave's mission](https://concave.dev/) to build unified infrastructure for the emerging "Internet of Agents." See also: [Adapt Fast In The AI Era](https://matmul.net/$/adapt-fast.html) and [writing code was never the bottleneck](https://ordep.dev/posts/writing-code-was-never-the-bottleneck).

**Key Features:**
- Sandboxed code execution (planned)
- Agent primitives (planned)

## Build

```bash
go build -o bin/prismd cmd/prismd/main.go
go build -o bin/prismctl cmd/prismctl/main.go
```

## Usage

Start the daemon:
```bash
# First node (bootstrap new cluster)
./bin/prismd --serf=0.0.0.0:4200 --bootstrap --name=first-node

# For production with remote API access (first node)
./bin/prismd --serf=0.0.0.0:4200 --api=0.0.0.0:8008 --bootstrap --name=first-node

# Join second node to existing cluster
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

## Implementation Status

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

## Known Issues

- **Raft peer lifecycle vs Serf membership:** Peers are added/removed by the Raft leader in response to Serf events. If this node is not the leader or events are dropped, stale peers can remain after a node leaves/fails until the leader reconciles.
- **Serf-based resource queries:** Cluster resource aggregation currently uses Serf `get-resources` distributed queries. Latency grows with cluster size and responses can be partial under timeouts; this is not a strongly consistent snapshot.
- **Resource aggregation delays on failures:** Aggregation counts all known members; failed nodes are included until reaped, so calls may wait for timeout before returning.
- **Security (no TLS/Auth):** HTTP API and gRPC are plaintext with no authentication/authorization; Serf gossip is unencrypted (no keyring configured).
- **Metrics completeness:** CPU usage, load averages, and job accounting are placeholders; reported metrics are not yet suitable for scheduling decisions.
- **Event backpressure risk:** External event forwarding can drop events when `ConsumerEventCh` is saturated, which may delay Raft peer reconciliation on non-leader nodes.
- **Serf reap delay:** Reaping of dead members is time-based. Large `DeadNodeReclaimTime` can delay full cleanup and UI/CLI visibility.
- **gRPC services TBD:** gRPC server runs but no services are registered yet; future resource APIs will move here.
- **Tag drift risk:** Raft/gRPC peer discovery relies on Serf tags (`raft_port`, `grpc_port`). Misconfigured/missing tags prevent peer wiring and may require manual correction.
- **Data at rest:** Raft logs/snapshots and local state are stored unencrypted by default in `./data`.
- **Minimum 3+ Nodes Required:** Clusters with fewer than 3 nodes lose fault tolerance and behave unpredictably. With 1 node: no fault tolerance (if it dies, cluster is down). With 2 nodes: no fault tolerance (if either dies, cluster loses quorum). **Recommendation:** Always deploy 3+ nodes (preferably odd numbers: 3, 5, 7) for meaningful fault tolerance.
- **Quorum Loss Recovery:** If majority of nodes fail, new nodes cannot join the cluster without manual intervention. The cluster will detect deadlock and provide recovery guidance, but operators must either restart dead nodes or rebuild from backup. Adding new nodes to a quorum-less cluster will fail with "rejecting pre-vote request since node is not in configuration" errors.
- **Node Identity vs Data Persistence:** Killed nodes that restart with the same data directory get fresh identities (new ID/name) but recover Raft log state. This prevents split-brain but means node identity is ephemeral while consensus state persists.
- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts. Use odd-numbered clusters (3+) for proper quorum.
- **Network Connectivity:** Nodes behind NAT or firewalls may fail to join. Ensure gossip ports are accessible between all nodes.
- **Bootstrap Race Conditions:** The current `--bootstrap` flag creates single-node clusters that become leader immediately, risking split-brain scenarios if multiple nodes bootstrap. **Recommendation:** Use `--bootstrap-expect=N` approach (like Nomad) where nodes wait for expected quorum before starting cluster formation.