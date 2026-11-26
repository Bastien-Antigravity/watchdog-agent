---
microservice: watchdog-agent
type: architecture
status: active
language: go
tags:
- '#ai/ignore'
- '#service/watchdog-agent'
- '#type/architecture'
- '#state/active'
- '#zone/3-fleet'
---
# 🏗️ Watchdog Agent — Architecture Overview

High-density structural map of the `watchdog-agent` process supervisor.

## Subsystem Architecture

The watchdog agent handles process management by walking configured dependencies, resolving target listening ports, compiling binaries, spawning child processes, and capturing and prefixing their logs. It also pings external components for ecosystem telemetry.

```mermaid
graph TD
    Launcher[watchdog-agent main] -->|1. File Lock| Lock[.watchdog-agent.lock]
    Launcher -->|2. Config Resolve| Config[AppConfig Capabilities]
    Launcher -->|3. Run Loop| Supervisor[Supervisory Threadpool]
    Launcher -->|4. Web Server| Server[REST API & embedded OpenMFE]
    Launcher -->|5. Error Shipper| Shipper[Autonomous Fleet Log Shipper]

    subgraph "Process Monitoring"
        Supervisor -->|Validate Topo| GraphCheck[Graph Deadlock/Cycle DFS]
        Supervisor -->|Wait for deps| DepCheck[Port Listening Check]
        Supervisor -->|Port Occupation| Pruner[Group Port Pruning]
        DepCheck -->|Healthy| Compile[Go / Cargo Compilation]
        Compile -->|Build OK| Exec[cmd.Start Subprocess]
        Exec -->|Pipe Logs| Logs[Prefix Logger]
    end

    subgraph "Autonomous Error Log Shipper"
        Shipper -->|Tail files| Tailer[Multi-File Tailer & Inode/Size Check]
        Tailer -->|Filter level >= ERROR| Parser[8-Column Zero-Alloc Parser]
        Parser -->|Atomic Batch Tx| DBHypertable[TimescaleDB fleet_error_logs]
        Shipper -->|Commit Offsets| CheckpointStore[.shipper_offsets.json]
    end

    subgraph "External Telemetry"
        Server -->|Status Telemetry| DBCheck[TimescaleDB ping]
        Server -->|Status Telemetry| RAGCheck[RAG MCP Server ping]
    end
```

## Repository Anatomy

The repository follows a clean, modular package layout:

```text
watchdog-agent/
├── main.go            # Agent startup bootstrap and config capability resolution
├── go.mod             # Module dependencies
├── quick-overview/    # Technical behavior documentation playbooks
└── src/
    ├── config/        # Environment configurations and filesystem symlink healing
    ├── control/       # NATS control plane integrations
    ├── core/          # Telemetry status struct definitions
    ├── rest/          # HTTP REST endpoints, cache headers, and OpenMFE dashboard
    ├── server/        # Controller implementation (live pings and status aggregation)
    ├── shipper/       # Autonomous error log tailer, zero-allocation parser, and TimescaleDB hypertable sink
    ├── supervisor/    # Supervisory loops, topological checks, and database launcher
    └── utils/         # Cross-platform single instance locks and group process killers
```
