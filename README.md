# Prism

Distributed runtime platform for AI agents and workflows.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://github.com/user-attachments/assets/3ee44401-f613-431d-811b-510b0f3c5b14">
  <source media="(prefers-color-scheme: light)" srcset="https://github.com/user-attachments/assets/1b42b694-eb66-4c6e-bf21-bbd2a22f23ab">
  <img alt="prismd screenshot" src="https://github.com/user-attachments/assets/3ee44401-f613-431d-811b-510b0f3c5b14">
</picture>

> [!CAUTION]
> This is early development software. Agent execution is not implemented yet. Test coverage: 7.8%.

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
- [x] **Resource Discovery**: Real-time system monitoring (CPU, memory, disk, load) across cluster nodes
- [x] **CLI Tooling**: Complete cluster management via `prismctl` with JSON/table output
- [x] **Network Resilience**: Graceful daemon lifecycle, automatic peer discovery, partition tolerance
- [x] **Autopilot & Reconciliation**: Automatic dead peer removal and cluster health maintenance

### Agent Execution
- [x] **Agent Lifecycle Management**: Complete CRUD operations for agents via REST API and CLI
- [x] **Agent State Management**: Distributed agent state tracking via Raft consensus (no actual execution)
- [ ] **Agent Orchestration**: Firecracker microVM lifecycle for secure agent isolation  
- [ ] **Serverless Scaling**: Dynamic scaling from 0 to 10k agent instances based on demand
- [ ] **Agent Scheduling**: Intelligent placement and execution of AI agents across cluster
- [ ] **Secure Sandboxing**: Isolated execution environments for safe agent code execution
- [ ] **Crash-Proof Execution**: Fault-tolerant agent runtime with automatic recovery and retry

### Agent Observability
- [x] **Health Monitoring**: Node health checks with gRPC and HTTP API endpoints
- [x] **Resource Metrics**: Real-time CPU, memory, disk, and load monitoring across cluster
- [x] **Cluster Visibility**: Comprehensive cluster status, member information, and Raft state
- [x] **API Endpoints**: RESTful APIs for health, resources, nodes, and cluster information
- [ ] **Agent Lifecycle Tracking**: Monitor agent execution, state transitions, and resource usage
- [ ] **Performance Analytics**: Real-time visibility into agent performance and microVM metrics
- [ ] **Agent Debugging**: Analysis and troubleshooting tools for distributed agent workloads

### Future Agent Infrastructure

- **Agent Memory & State**: Distributed memory and persistent state management for agents
- **Agent Mesh**: Inter-agent communication and service discovery with first-class MCP support
- **Multi-Runtime Support**: cloud-hypervisor, Docker containers for different agent types
- **Agent Workflows**: Durable execution patterns with automatic retry and recovery
- **LLM Gateway**: Built-in LLM routing, rate limiting, and cost optimization
- **Secrets Vault**: Secure credential management for agent API access

## Quick Start

Get a 3-node cluster running locally in under 30 seconds:

```bash
# Build binaries
make build

# Start 3-node cluster on localhost
make run

# Check cluster status
./bin/prismctl info
```

## Build

```bash
# Using Make
make build

# Or using Go directly
go build -o bin/prismd ./cmd/prismd
go build -o bin/prismctl ./cmd/prismctl
```

## Usage

### Development and Testing

For development and testing, you can omit port configuration. The system auto-discovers services and configures ports automatically:

```bash
# Bootstrap the first node - auto-configures all ports and data directory
./bin/prismd --bootstrap

# Enable debug mode for development (verbose logging and detailed HTTP output)
DEBUG=true ./bin/prismd --bootstrap

# Join other nodes - only need to specify serf address to join
./bin/prismd --join=192.168.1.100:4200
```

### Production Deployment

For production use, **explicitly assign all ports and data directory** for predictable service endpoints. Use `--bootstrap-expect` for safe cluster formation. **Use odd-numbered clusters (3, 5, 7) for proper quorum and fault tolerance.**

```bash
# Production-safe 3-node cluster formation
# All nodes use the same bootstrap-expect value and join addresses

# Node 1 - API exposed for management (ensure firewall protection)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --raft=0.0.0.0:4201 \
  --grpc=0.0.0.0:4202 \
  --api=0.0.0.0:8008 \
  --data-dir=/var/lib/prism \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-1

# Node 2 - API on localhost only (more secure, requires SSH tunnel for remote access)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --raft=0.0.0.0:4201 \
  --grpc=0.0.0.0:4202 \
  --api=127.0.0.1:8008 \
  --data-dir=/var/lib/prism \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-2

# Node 3 - API on localhost only (more secure, requires SSH tunnel for remote access)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --raft=0.0.0.0:4201 \
  --grpc=0.0.0.0:4202 \
  --api=127.0.0.1:8008 \
  --data-dir=/var/lib/prism \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-3
```

**Legacy Single-Node Bootstrap** (development only):
```bash
# Single node with immediate bootstrap (split-brain risk in production)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --raft=0.0.0.0:4201 \
  --grpc=0.0.0.0:4202 \
  --api=127.0.0.1:8008 \
  --data-dir=/var/lib/prism \
  --bootstrap \
  --name=dev-node-1
```

**Bootstrap-Expect Behavior**: 
- All nodes wait until exactly N peers are discovered via Serf gossip
- Only one node (deterministically selected by smallest node ID) coordinates cluster formation  
- Prevents split-brain scenarios during concurrent startup
- Includes stability window to handle network churn during bootstrap
- Use `--bootstrap-expect=1` for single-node development clusters

**Auto-Configuration**: When ports are not explicitly specified, the system automatically configures available ports and uses timestamped directories like `./data/20240115-143022` for storage. All services (Raft, gRPC, API) auto-discover each other through Serf gossip.

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

## Security Considerations

### Encryption & Transport Security

- **No TLS/HTTPS**: All HTTP API communication is unencrypted plaintext
- **No gRPC TLS**: Inter-node gRPC communication is unencrypted 
- **No Serf Keyring**: Cluster gossip protocol is unencrypted (no keyring configured)
- **No Raft Transport Encryption**: Consensus protocol communication is unencrypted

**Current Risk**: All network traffic (API requests, inter-node communication, cluster gossip) can be intercepted and read by network attackers.

**Mitigation**: Deploy only on trusted networks (private VLANs, VPNs) until TLS support is implemented.

### API Security

The HTTP API defaults to `127.0.0.1:8008` (localhost only) as a security-first approach to prevent accidental exposure to external networks. This is intentional since the API currently has no authentication or authorization mechanisms (authentication will be implemented soon).

**Local Management (Default - Secure)**:
```bash
# API only accessible from localhost
./bin/prismd  # API binds to 127.0.0.1:8008
./bin/prismctl info  # Connects to 127.0.0.1:8008
```

**Remote Management (Explicit - Use with Caution)**:
```bash
# Explicitly expose API to all interfaces (production requires firewall/VPN)
./bin/prismd --api=0.0.0.0:8008

# Connect from remote machine
./bin/prismctl --api=192.168.1.100:8008 info
```

**Production Deployment**: When exposing the API externally (`--api=0.0.0.0:8008`), ensure proper network security:
- Use firewall rules to restrict API access to trusted networks
- Deploy behind VPN or use SSH tunneling for remote access  
- Consider using reverse proxy with authentication (nginx, traefik)

### Planned Security Improvements

- **TLS/HTTPS**: Encrypted HTTP API with certificate management
- **gRPC TLS**: Encrypted inter-node communication with mutual TLS
- **Serf Keyring**: Encrypted cluster gossip with pre-shared keys
- **Authentication**: API authentication and authorization mechanisms
- **Raft Transport Security**: Encrypted consensus protocol communication

## Known Issues

- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts.
- **Quorum Loss Recovery:** If majority of nodes fail, new nodes cannot join the cluster without manual intervention. The cluster will detect deadlock and provide recovery guidance, but operators must either restart dead nodes or rebuild from backup. Adding new nodes to a quorum-less cluster will fail with "rejecting pre-vote request since node is not in configuration" errors.
- **Resource aggregation delays on failures:** Aggregation counts all known members; failed nodes are included until reaped, so calls may wait for timeout before returning.
- **Serf reap delay:** Reaping of dead members is time-based. Large `DeadNodeReclaimTime` can delay full cleanup and UI/CLI visibility.
- **Metrics completeness:** CPU usage, load averages, and job accounting are placeholders; reported metrics are not yet suitable for scheduling decisions.
- **0.0.0.0 Bind Address Limitation:** When `0.0.0.0` is specified as a bind address, the system doesn't truly bind to all network interfaces. Instead, it resolves to a single IP address by determining which local interface can reach external networks (via 8.8.8.8). This works for most setups but may bind to unexpected interfaces on multi-homed servers or fail in offline environments where it falls back to 127.0.0.1.
- **IPv6 Not Supported:** The system currently only supports IPv4 addresses. IPv6 addresses, dual-stack configurations, and IPv6-only environments are not supported. All network components (Serf, Raft, gRPC, HTTP API) assume IPv4 addressing.
