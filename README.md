# 🛰️ watchdog-agent

Resilient process supervisor, cross-platform lifecycle manager, and telemetry coordinator for the Bastien-Antigravity microservice ecosystem.

## 🚀 Overview

The `watchdog-agent` is a modular, cross-platform Go-based supervisor that coordinates the compilation, startup, dependency verification, port clearing, and log redirection of all local fleet microservices. It also exposes a REST management API and embeds a custom OpenMFE web component dashboard allowing developers to monitor and control the ecosystem in real-time.

## 🛠️ Key Features

- **Process Supervision**: Supervises child processes, capturing stdout/stderr and routing them with service-specific prefixes and colors.
- **Dependency Topological Ordering**: Validates dependency graphs at startup to detect loop deadlocks and missing services, blocking process launch until sibling dependencies are healthy.
- **Multi-Node Autonomous Filtering**: Uses `utils.IsLocal(svc.IP)` to automatically supervise only services assigned to the local node, skipping remote nodes (cloud VPS or external machines) without requiring distinct configs.
- **Dynamic NATS & Bus Management**: Dynamically launches and binds `nats-server` to configured IPs and ports (`-a <ip> -p <port> -c <config> -js`), maintaining parity across native and container stacks.
- **Cross-Platform Port Pruning**: Automatically detects and terminates orphaned processes occupying target ports using native group/tree termination (Unix process groups and Windows `taskkill`).
- **Telemetry Integration**: Periodically pings external dependencies (**TimescaleDB**, **NATS**, and **RAG MCP Server**) to verify connectivity, reporting statuses on the dashboard.
- **Dynamic Tele-Remote Bus**: Connects to `tele-remote` on port `1863` via gRPC to publish dynamic menus and bot controls.
- **Auto-Boot & Remote DB Launch**: Supports triggering cross-platform database startups (Docker, macOS Brew, Linux systemd, Windows Services) both automatically at boot and on-demand from the web UI.
- **OpenMFE Web Dashboard**: Serves a dynamic loader script with integrated cache-busting and client-side reload headers.
- **Single-Instance Locking**: Obtains a secure file lock on `.watchdog-agent.lock` using Unix `Flock` or Windows exclusive `CreateFile` calls.
- **Autonomous Fleet Error Log Shipper**: Background tailer that autonomously discovers microservice log files across Docker volumes (`/var/log/fleet`) and native workspaces (`*/logs/*.log`), parses standard 8-column log lines with zero heap allocations, filters entries with severity `level >= ERROR`, and ingests them in batch transactions into the TimescaleDB `fleet_error_logs` hypertable. Maintains checkpoint persistence in `.shipper_offsets.json` with file truncation/rotation detection and offset-freeze resilience during database downtime.

## 📖 Documentation

- [[quick-overview/Architecture-Overview.md]]: System topology, modular subsystem map, and directory layout.
- [[quick-overview/Features-Behavior.md]]: Process lifecycle loop details, database auto-launch, and telemetry checks.
- [[quick-overview/Testing-Playbook.md]]: Validation scripts and manual steps.
- [[AI-Project-DNA.md]]: AI squad behavioral contract and architectural boundaries.
- [[AI-Init.md]]: Onboarding guidelines for agents.

## 🏗️ Getting Started

### Run Watchdog Agent

```bash
go run main.go --profile standalone --build=true
```

Flags:

- `--profile`: Configuration profile to load (default: `standalone`).
- `--build`: If `true`, builds the binaries (e.g., `cargo build`, `go build`) before running them.
- `--mode`: Operational mode: `all` (default native), `supervisor` (process manager only), `shipper` (error log shipper only), `telemetry` (REST/MFE only). Defaults to `shipper` inside Docker containers (`DOCKER_ENV=true`).
- `--supervise`: Boolean flag to enable process supervision and auto-restart loops (default `true` on native host, automatically `false` in Docker where Docker daemon manages container lifecycles).
- `--shipper`: Boolean flag to enable the autonomous error log shipper to TimescaleDB (default `true`).
- `--rest`: Boolean flag to enable the HTTP REST API and embedded OpenMFE dashboard (default `true` in native, `false` in standalone shipper mode).

Environment Variable Overrides:

- `WATCHDOG_MODE`: Set mode (`all`, `supervisor`, `shipper`, `telemetry`).
- `WATCHDOG_SUPERVISE`: Explicitly toggle process supervision (`true`/`false`).
- `WATCHDOG_SHIPPER`: Explicitly toggle log shipping (`true`/`false`).
- `WATCHDOG_REST`: Explicitly toggle REST API (`true`/`false`).
- `LOG_SCAN_DIRS`: Override log directory to scan (e.g., `/var/log/fleet`).

---

*Version: v0.0.1 (See main.go)*
