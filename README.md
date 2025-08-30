# Prism

Open-Source distributed sandbox runtime for running AI-generated code.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://github.com/user-attachments/assets/7a0bb8f0-5641-4e72-b9df-ddfd64a8da0d">
  <source media="(prefers-color-scheme: light)" srcset="https://github.com/user-attachments/assets/56fabde0-0e37-4f0b-b335-bd5372aa1177">
  <img alt="prismd screenshot" src="https://github.com/user-attachments/assets/7a0bb8f0-5641-4e72-b9df-ddfd64a8da0d">
</picture>

> [!CAUTION]
> Early development software. Cluster management and v0 scheduler are complete, 
> but runtime execution is not wired to Firecracker/Docker yet.

## Overview  

Prism is a high-performance distributed sandbox runtime designed specifically for running AI-generated code. Built on battle-tested [Firecracker](https://firecracker-microvm.github.io/) microVMs, it provides strong isolation without the overhead of traditional containers.

- **Security-First Architecture**: True isolation via dedicated kernels with minimal attack surface.
- **Horizontal Scalability**: Distributed system that scales across multiple nodes.
- **Persistent Storage**: Durable volumes ensure sandboxes can be reused across executions.
- **High Performance**: Firecracker's lightweight virtualization starts sandboxes in few hundred milliseconds.
- **Docker Compatibility**: Supports any Docker image via OCI compliance.
- **Multi-Runtime Flexibility**: Firecracker for production isolation, Docker for development.

## Implementation Status

### What's Done
- [x] **Multi-node cluster** - Nodes can join, discover each other, and handle failures
- [x] **Leader election** - Automatic leader selection and failover
- [x] **Resource monitoring** - Real-time CPU, memory, disk tracking across nodes
- [x] **CLI management** - Complete cluster control via `prismctl` commands
- [x] **REST API** - HTTP endpoints for all cluster operations
- [x] **Sandbox state tracking** - Create, list, destroy, exec sandboxes with distributed state
- [x] **V0 distributed scheduler** - gRPC-based placement with leader verification and simulated responses

### What's Next
- [ ] **Runtime integration** - Connect to Firecracker/Docker for actual execution
- [ ] **Code execution** - Actually run commands inside sandbox environments
- [ ] **File operations** - Copy files in/out, stream logs, shell access
- [ ] **Network isolation** - Per-sandbox networking and security policies
- [ ] **Image management** - Pull container images and create snapshots

## Quick Start

Get a 3-node cluster running locally in under 30 seconds:

```bash
# Build binaries
make build

# Start 3-node cluster on localhost
make run

# Check cluster status
./bin/prismctl info

# Try sandbox commands
./bin/prismctl sandbox create --name=test
./bin/prismctl sandbox ls
./bin/prismctl sandbox info test
```

## Usage

### Development and Testing

For development and testing, you can omit port configuration. The system auto-discovers services and configures ports automatically:

```bash
# Bootstrap the first node - auto-configures all ports and data directory
./bin/prismd --bootstrap

# (optional) Enable debug mode for development (verbose logging and detailed HTTP output)
DEBUG=true ./bin/prismd --bootstrap

# Join other nodes - only need to specify cluster address to join
./bin/prismd --join=192.168.1.100:4200 # bootstrap node address
```

**Auto-Configuration**: When ports are not explicitly specified, the system automatically configures available ports and uses timestamped directories like `./data/20240115-143022` for storage. All services (Raft, gRPC, API) auto-discover each other through Serf gossip.

### Production Deployment

For production use, Use `--bootstrap-expect` for safe cluster formation. **Use odd-numbered clusters (3, 5, 7) for proper quorum and fault tolerance.**

```bash
# Production-safe 3-node cluster formation
# All nodes use the same bootstrap-expect value and join addresses

# Node 1 - API exposed for management (ensure firewall protection)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-1

# Node 2 - API inherits Serf address (enables leader forwarding)
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-2

# Node 3 - API inherits Serf address (enables leader forwarding)  
./bin/prismd \
  --serf=0.0.0.0:4200 \
  --bootstrap-expect=3 \
  --join=10.0.1.100:4200,10.0.1.101:4200,10.0.1.102:4200 \
  --name=prod-node-3
```

Use the CLI:
```bash
# Show cluster overview
./bin/prismctl info

# List all nodes with status
./bin/prismctl node ls

# Get detailed node information
./bin/prismctl node info <node-name-or-id>

# Manage code execution sandboxes
./bin/prismctl sandbox create --name=my-sandbox
./bin/prismctl sandbox ls
./bin/prismctl sandbox info <sandbox-name-or-id>
./bin/prismctl sandbox exec <sandbox-name-or-id> --command="python -c 'print(42)'"
./bin/prismctl sandbox logs <sandbox-name-or-id>
./bin/prismctl sandbox stop <sandbox-name-or-id>
./bin/prismctl sandbox destroy <sandbox-name-or-id>

# Connect to remote cluster
./bin/prismctl --api=192.168.46.110:8008 info

# Enable debug output for CLI operations
DEBUG=true ./bin/prismctl info
```

**Safety Note**: Commands like `exec`, `stop`, and `destroy` require exact ID or name matches - no partial matching for safety.

## Security Considerations

### Encryption & Transport Security

- **No TLS/HTTPS**: All HTTP API communication is unencrypted plaintext
- **No gRPC TLS**: Inter-node gRPC communication is unencrypted 
- **No Serf Keyring**: Cluster gossip protocol is unencrypted (no keyring configured)
- **No Raft Transport Encryption**: Consensus protocol communication is unencrypted

**Mitigation**: Deploy only on trusted networks (private VLANs, VPNs) until TLS support is implemented.

### API Security

The HTTP API defaults to cluster-wide accessibility (inherits Serf bind address) to enable transparent leader forwarding across nodes. The system includes automatic request routing where write operations sent to any node are transparently forwarded to the current Raft leader.

**Cluster-Wide Access (Default)**:
```bash
# Connect to any node - requests automatically routed to leader node
./bin/prismctl --api=192.168.1.100:8008 sandbox create --name=test
./bin/prismctl --api=192.168.1.101:8008 sandbox create --name=test2  # Same result
```

**Localhost-Only (Explicit - More Secure)**:
```bash
# Restrict API to localhost only (naturally disables leader forwarding from other nodes)
./bin/prismd --api=127.0.0.1:8008

# Only local access possible
./bin/prismctl --api=127.0.0.1:8008 info
```

**Production Deployment**: Since the API defaults to cluster-wide access for operational convenience, ensure proper network security:
- Use firewall rules to restrict API access to trusted networks
- Deploy behind VPN or use SSH tunneling for remote access  
- Consider using reverse proxy with authentication (nginx, traefik)
- Use `--api=127.0.0.1:8008` on non-management nodes for additional security

## Known Issues

- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts.
- **Quorum Loss Recovery:** If majority of nodes fail, new nodes cannot join the cluster without manual intervention. The cluster will detect deadlock and provide recovery guidance, but operators must either restart dead nodes or rebuild from backup. Adding new nodes to a quorum-less cluster will fail with "rejecting pre-vote request since node is not in configuration" errors.
- **Resource aggregation delays on failures:** Aggregation counts all known members; failed nodes are included until reaped, so calls may wait for timeout before returning.
- **Serf reap delay:** Reaping of dead members is time-based. Large `DeadNodeReclaimTime` can delay full cleanup and UI/CLI visibility.
- **Metrics completeness:** CPU usage, load averages, and job accounting are placeholders; reported metrics are not yet suitable for scheduling decisions.
- **V0 scheduler simulation:** Placement and stop operations return simulated responses (random success/failure) to exercise distributed scheduling patterns before runtime integration.
- **0.0.0.0 Bind Address Limitation:** When `0.0.0.0` is specified as a bind address, the system doesn't truly bind to all network interfaces. Instead, it resolves to a single IP address by determining which local interface can reach external networks (via 8.8.8.8). This works for most setups but may bind to unexpected interfaces on multi-homed servers or fail in offline environments where it falls back to 127.0.0.1.
- **IPv6 Not Supported:** The system currently only supports IPv4 addresses. IPv6 addresses, dual-stack configurations, and IPv6-only environments are not supported. All network components (Serf, Raft, gRPC, HTTP API) assume IPv4 addressing.
