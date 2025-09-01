# Quick Start

## Local Development

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

## Development and Testing

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

## Production Deployment

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

## Using the CLI

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
./bin/prismctl sandbox rm <sandbox-name-or-id>

# Connect to remote cluster
./bin/prismctl --api=192.168.46.110:8008 info

# Enable debug output for CLI operations
DEBUG=true ./bin/prismctl info
```

**Safety Note**: Commands like `exec`, `stop`, and `rm` require exact ID or name matches - no partial matching for safety.
