# Prism

Distributed runtime platform for AI agents.

## Build

```bash
go build -o bin/prismd cmd/prismd/main.go
go build -o bin/prismctl cmd/prismctl/main.go
```

## Usage

Start the daemon:
```bash
./bin/prismd --bind=0.0.0.0:4200 --role=control --node-name=first-node
./bin/prismd --join=127.0.0.1:4200 --bind=127.0.0.1:4201 --node-name=second-node --role=agent
```

Use the CLI:
```bash
./bin/prismctl members
./bin/prismctl status
```