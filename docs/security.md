# Security Considerations

## Encryption & Transport Security

- **No TLS/HTTPS**: All HTTP API communication is unencrypted plaintext
- **No gRPC TLS**: Inter-node gRPC communication is unencrypted 
- **No Serf Keyring**: Cluster gossip protocol is unencrypted (no keyring configured)
- **No Raft Transport Encryption**: Consensus protocol communication is unencrypted

**Mitigation**: Deploy only on trusted networks (private VLANs, VPNs) until TLS support is implemented.

## API Security

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
