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
./bin/prismd --bind=0.0.0.0:4200 --name=first-node

# For production with remote API access
./bin/prismd --bind=0.0.0.0:4200 --api=0.0.0.0:8020 --name=first-node

# Join second node 
./bin/prismd --join=127.0.0.1:4200 --bind=127.0.0.1:4201 --name=second-node
```

Use the CLI:
```bash
./bin/prismctl members
./bin/prismctl status
```


## Known Issues

- **Network Connectivity:** Nodes behind NAT or firewalls may fail to join. Ensure the gossip (bind) port is accessible between all nodes.
- **Split Brain:** Network partitions can result in multiple control nodes. Monitor cluster health and use proper quorum mechanisms.
- **Resource Synchronization:** Resource state may be inconsistent during network interruptions. Eventual consistency checks are recommended.
- **No Consensus:** All data is fetched in real-time from Serf without a consensus mechanism, leading to potentially inconsistent cluster state views.
- **Cluster Sizing:** Use odd numbers of nodes (3, 5, 7, etc.) for optimal stability and conflict resolution. Avoid 2-node clusters due to instability from name conflicts.