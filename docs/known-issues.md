# Known Issues

- **Split Brain Risk:** Network partitions can cause Raft leadership conflicts.
- **Quorum Loss Recovery:** If majority of nodes fail, new nodes cannot join the cluster without manual intervention. The cluster will detect deadlock and provide recovery guidance, but operators must either restart dead nodes or rebuild from backup. Adding new nodes to a quorum-less cluster will fail with "rejecting pre-vote request since node is not in configuration" errors.
- **Resource aggregation delays on failures:** Aggregation counts all known members; failed nodes are included until reaped, so calls may wait for timeout before returning.
- **Serf reap delay:** Reaping of dead members is time-based. Large `DeadNodeReclaimTime` can delay full cleanup and UI/CLI visibility.
- **Metrics completeness:** CPU usage, load averages, and job accounting are placeholders; reported metrics are not yet suitable for scheduling decisions.
- **V0 scheduler simulation:** Placement and stop operations return simulated responses (random success/failure) to exercise distributed scheduling patterns before runtime integration.
- **0.0.0.0 Bind Address Limitation:** When `0.0.0.0` is specified as a bind address, the system doesn't truly bind to all network interfaces. Instead, it resolves to a single IP address by determining which local interface can reach external networks (via 8.8.8.8). This works for most setups but may bind to unexpected interfaces on multi-homed servers or fail in offline environments where it falls back to 127.0.0.1.
- **IPv6 Not Supported:** The system currently only supports IPv4 addresses. IPv6 addresses, dual-stack configurations, and IPv6-only environments are not supported. All network components (Serf, Raft, gRPC, HTTP API) assume IPv4 addressing.
- **Additional Issues:** You'll find more issues just by searching TODOs throughout the codebase.
