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

## Quick Start

Get started with Prism in minutes. See [docs/quickstart.md](docs/quickstart.md) for local development, testing, and production deployment instructions.

## Security

Prism currently has security limitations including unencrypted communication and no authentication. For detailed security information, deployment recommendations, and mitigation strategies, see [docs/security.md](docs/security.md).

## Known Issues

For current limitations and known issues, see [docs/known-issues.md](docs/known-issues.md).
