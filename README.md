# Prism

> [!CAUTION]
> The cluster is not yet stable. Use at your own risk.

Distributed runtime platform for AI agents.

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
./bin/prismd --join=0.0.0.0:4200 --serf=0.0.0.0:4201 --name=second-node
```

Use the CLI:
```bash
./bin/prismctl members
./bin/prismctl status
```


## Known Issues

- **Missing Task/VM Scheduling:** No task scheduler or VM orchestration implemented yet. Currently focused on distributed systems foundations (membership, consensus, API).
- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts. Use odd-numbered clusters (3+) for proper quorum.
- **Network Connectivity:** Nodes behind NAT or firewalls may fail to join. Ensure gossip ports are accessible between all nodes.
- **Resource Query Timeouts:** Resource collection via Serf queries may timeout in slow networks or large clusters (>50 nodes).
- **Limited Raft Integration:** Raft consensus is present but not yet used for critical cluster state beyond basic peer management.