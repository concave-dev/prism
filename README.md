# Prism

Distributed runtime platform for AI agents and workflows.

![prismd screenshot](https://github.com/user-attachments/assets/15fb5429-0bd0-4a1e-9b3c-0579f76608b2)

> [!CAUTION]
> The cluster is not yet stable. Use at your own risk. Test coverage: ~9%.

## Overview

Distributed orchestration platform for AI agents with adaptive scaling, fault-tolerant execution, and multi-runtime support.

Designed specifically for AI workloads, Prism addresses unique operational requirements including non-deterministic behavior, dynamic resource scaling, and long-running probabilistic workflows. The platform supports multiple execution runtimes: Firecracker microVMs for secure isolation, cloud-hypervisor for performance optimization, and Docker containers for development workflows.

The AI ecosystem is fragmented with slow deployment cycles. Current infrastructure wasn't designed for autonomous agents that need rapid iteration, inter-agent communication, and built-in observability. Prism aims to compress the time from idea to deployed agent from weeks to minutes.

This aligns with [Concave's mission](https://concave.dev/) to build unified infrastructure for the emerging _"Internet of Agents"_. See also: [Adapt Fast In The AI Era](https://matmul.net/$/adapt-fast.html) and [writing code was never the bottleneck](https://ordep.dev/posts/writing-code-was-never-the-bottleneck).

## Features & Implementation Status

### Distributed Control Plane
- [x] **Cluster Management**: Serf gossip protocol for membership, failure detection, auto-discovery
- [x] **Consensus Layer**: Raft for leader election, log replication, strong consistency guarantees  
- [x] **Inter-Node Communication**: gRPC NodeService with HTTP REST API fallback
- [x] **Resource Discovery**: Real-time system monitoring (CPU, memory, load) across cluster nodes
- [x] **CLI Tooling**: Complete cluster management via `prismctl` with JSON/table output
- [x] **Network Resilience**: Graceful daemon lifecycle, automatic peer discovery, partition tolerance

### Workload Execution
- [ ] **MicroVM Orchestration**: Firecracker VM lifecycle management with sub-second boot times
- [ ] **Burst Scaling**: Dynamic scaling from 0 to 10k instances based on workload demands
- [ ] **Task Scheduling**: Resource-aware placement and execution of agent workloads
- [ ] **Sandbox Isolation**: Secure execution environments for code execution

### Observability
- [ ] **Execution Tracking**: Monitor workload lifecycle and resource utilization
- [ ] **Performance Metrics**: Real-time visibility into VM and cluster performance
- [ ] **Debugging Tools**: Analysis and troubleshooting for distributed workloads

### Future Agentic Primitives

- Distributed memory and state management
- Agent mesh and inter-agent communication  
- Multi-runtime support (cloud-hypervisor, Docker)
- Crash-proof agent execution

## Build

```bash
go build -o bin/prismd ./cmd/prismd
go build -o bin/prismctl ./cmd/prismctl
```

## Usage

Start the daemon:
```bash
# First node (bootstrap new cluster) - auto-configures ports and data directory
./bin/prismd --bootstrap --name=first-node

# Join second node to existing cluster - auto-configures ports and data directory  
./bin/prismd --join=192.168.1.100:4200 --name=second-node

# Explicit configuration for production (advanced usage)
./bin/prismd --serf=0.0.0.0:4200 --api=0.0.0.0:8008 --data-dir=/var/lib/prism --bootstrap --name=first-node

# Enable debug mode for development (verbose logging and detailed HTTP output)
DEBUG=true ./bin/prismd --bootstrap --name=first-node
```

**Auto-Configuration**: Network addresses (`--serf`, `--raft`, `--grpc`, `--api`) and data directory (`--data-dir`) are automatically configured when not explicitly specified, using available ports and timestamped directories like `./data/20240115-143022` for storage.

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

# Enable debug output for CLI operations
DEBUG=true ./bin/prismctl info
```

## Known Issues

- **Security (no TLS/Auth):** HTTP API and gRPC are plaintext with no authentication/authorization; Serf gossip is unencrypted (no keyring configured).
- **Bootstrap Race Conditions:** The current `--bootstrap` flag creates single-node clusters that become leader immediately, risking split-brain scenarios if multiple nodes bootstrap. **Recommendation:** Adopt a bootstrap-expect-style approach where nodes wait for expected quorum before forming a cluster — planned; not yet implemented.
- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts. Use odd-numbered clusters (3+) for proper quorum.
- **Quorum Loss Recovery:** If majority of nodes fail, new nodes cannot join the cluster without manual intervention. The cluster will detect deadlock and provide recovery guidance, but operators must either restart dead nodes or rebuild from backup. Adding new nodes to a quorum-less cluster will fail with "rejecting pre-vote request since node is not in configuration" errors.
- **Minimum 3+ Nodes Required:** Clusters with fewer than 3 nodes lose fault tolerance and behave unpredictably. With 1 node: no fault tolerance (if it dies, cluster is down). With 2 nodes: no fault tolerance (if either dies, cluster loses quorum). **Recommendation:** Always deploy 3+ nodes (preferably odd numbers: 3, 5, 7) for meaningful fault tolerance.
- **Raft peer lifecycle vs Serf membership:** Peers are added/removed by the Raft leader in response to Serf events. If this node is not the leader or events are dropped, stale peers can remain after a node leaves/fails until the leader reconciles.
- **Tag drift risk:** Raft/gRPC peer discovery relies on Serf tags (`raft_port`, `grpc_port`). Misconfigured/missing tags prevent peer wiring and may require manual correction.
- **Node Identity vs Data Persistence:** Killed nodes that restart with the same data directory get fresh identities (new ID/name) but recover Raft log state. This prevents split-brain but means node identity is ephemeral while consensus state persists.
- **Resource aggregation delays on failures:** Aggregation counts all known members; failed nodes are included until reaped, so calls may wait for timeout before returning.
- **Serf reap delay:** Reaping of dead members is time-based. Large `DeadNodeReclaimTime` can delay full cleanup and UI/CLI visibility.
- **Resource queries (gRPC-first):** Cluster resource aggregation now queries each node via gRPC for speed and consistency, with Serf `get-resources` as a fallback if gRPC fails. Gossip fallback can be slower and partial under timeouts; not a strongly consistent snapshot.
- **Network Connectivity:** Nodes behind NAT or firewalls may fail to join. Ensure gossip ports are accessible between all nodes.
- **Metrics completeness:** CPU usage, load averages, and job accounting are placeholders; reported metrics are not yet suitable for scheduling decisions.
- **0.0.0.0 Bind Address Limitation:** When `0.0.0.0` is specified as a bind address, the system doesn't truly bind to all network interfaces. Instead, it resolves to a single IP address by determining which local interface can reach external networks (via 8.8.8.8). This works for most setups but may bind to unexpected interfaces on multi-homed servers or fail in offline environments where it falls back to 127.0.0.1.
- **IPv6 Not Supported:** The system currently only supports IPv4 addresses. IPv6 addresses, dual-stack configurations, and IPv6-only environments are not supported. All network components (Serf, Raft, gRPC, HTTP API) assume IPv4 addressing.
