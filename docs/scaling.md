# Scaling Guide

Comprehensive guide for scaling Prism clusters from development to production deployments.

## Cluster Sizing Recommendations

### Node Count Guidelines

**Odd Numbers Preferred**: Use odd-numbered cluster sizes (3, 5, 7+) for optimal Raft consensus and conflict resolution.

- **Single Node**: Development only, no fault tolerance
- **3 Nodes**: Minimum production deployment, survives 1 node failure
- **5 Nodes**: Recommended for production, survives 2 node failures  
- **7+ Nodes**: Large deployments, higher fault tolerance

**Maximum Cluster Size**: Keep clusters under 10 nodes for optimal performance. Raft consensus overhead increases with cluster size.

## Performance Tuning

Prism includes intelligent request batching for high-throughput scenarios. Batching is **enabled by default** and automatically switches between pass-through and batching modes based on load.

**Default Settings** (optimized for high-throughput scenarios):
```bash
--batching-enabled=true          # Enable smart batching (default)
--create-queue-size=12000        # Create queue capacity (validated with 10k sandbox stress test)
--delete-queue-size=22000        # Delete queue capacity  
--batch-threshold=50             # Start batching when queue > 50 items (conservative trigger)
--batch-interval-ms=250          # Start batching when requests < 250ms apart (relaxed timing)
```

## Scaling Patterns

**Recommended: Vertical Scaling First**

Scale vertically before adding nodes - fewer nodes reduce Raft consensus overhead and operational complexity:

- **CPU**: Increase cores for higher scheduling throughput
- **Memory**: Add RAM for larger sandbox capacity  
- **Storage**: Use faster storage (NVMe) for better I/O performance
- **Network**: Upgrade to higher bandwidth interfaces

**Horizontal Scaling** (when vertical limits reached):
- Add nodes in odd increments only (3→5→7)
- Each additional node increases Raft consensus latency
- Distribute load across multiple API endpoints
- Use load balancers for client connection distribution

## Deployment Patterns

### Development Clusters

**Single Node**:
```bash
# Bootstrap single-node cluster
prismd --bootstrap --name=dev-node
```

### Production Clusters

**Multi-Node Clusters**:
```bash
# 3-Node cluster (all nodes start simultaneously)
prismd --bootstrap-expect=3 --join=node1:4200,node2:4200,node3:4200 --name=node1
prismd --bootstrap-expect=3 --join=node1:4200,node2:4200,node3:4200 --name=node2
prismd --bootstrap-expect=3 --join=node1:4200,node2:4200,node3:4200 --name=node3

# 5-Node cluster (all nodes start simultaneously)
prismd --bootstrap-expect=5 --join=node1:4200,node2:4200,node3:4200,node4:4200,node5:4200 --name=node1
prismd --bootstrap-expect=5 --join=node1:4200,node2:4200,node3:4200,node4:4200,node5:4200 --name=node2
prismd --bootstrap-expect=5 --join=node1:4200,node2:4200,node3:4200,node4:4200,node5:4200 --name=node3
prismd --bootstrap-expect=5 --join=node1:4200,node2:4200,node3:4200,node4:4200,node5:4200 --name=node4
prismd --bootstrap-expect=5 --join=node1:4200,node2:4200,node3:4200,node4:4200,node5:4200 --name=node5
```

### Load Balancing

**API Access**: Clients can connect to any node - leader forwarding handles write operations automatically.

```bash
# Any node works for API access
prismctl --api=node1:8008 sandbox create
prismctl --api=node2:8008 sandbox create  
prismctl --api=node3:8008 sandbox create
```

## Troubleshooting Common Issues

### Network Port Exhaustion

**Symptoms**:
- "can't assign requested address" errors
- Raft heartbeat failures
- Intermittent leader elections

**Solutions**:
- Increase system file descriptor limits
- Tune network stack parameters
- Reduce cluster size if necessary
- Implement connection pooling

### Raft Consensus Issues

**Symptoms**:
- Raft commit latency
- Frequent leader elections
- "No Raft leader available" errors
- Split-brain scenarios

**Solutions**:
- Ensure odd cluster sizes
- Verify network connectivity between all nodes
- Check for network partitions
- Monitor Raft peer connectivity
- Smaller cluster

## Future Scaling Considerations

As Prism evolves toward its full vision of an AI orchestration platform, scaling patterns will expand to include:

- Multi-region deployments
- Cross-cluster communication
- Advanced resource management
- Auto-scaling capabilities

## Best Practices

1. **Start Small**: Begin with 3-node clusters and scale up based on demand
2. **Monitor Actively**: Use watch commands and external monitoring
3. **Plan Capacity**: Account for burst traffic and peak workloads
4. **Test Thoroughly**: Validate scaling changes in staging environments
5. **Document Changes**: Track configuration changes and performance impacts
