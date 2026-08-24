# 🛰️ watchdog-agent

Resilient process supervisor, cross-platform lifecycle manager, and telemetry coordinator for the Bastien-Antigravity microservice ecosystem.

## 🚀 Overview

The `watchdog-agent` is a modular, cross-platform Go-based supervisor that coordinates the compilation, startup, dependency verification, port clearing, and log redirection of all local fleet microservices. It also exposes a REST management API and embeds a custom OpenMFE web component dashboard allowing developers to monitor and control the ecosystem in real-time.

## 🛠️ Key Features

- **Process Supervision**: Supervises child processes, capturing stdout/stderr and routing them with service-specific prefixes and colors.
- **Dependency Topological Ordering**: Validates dependency graphs at startup to detect loop deadlocks and missing services, blocking process launch until sibling dependencies are healthy.
- **Cross-Platform Port Pruning**: Automatically detects and terminates orphaned processes occupying target ports using native group/tree termination (Unix process groups and Windows `taskkill`).
- **Telemetry Integration**: Periodically pings external dependencies (**TimescaleDB** and **RAG MCP Server**) to verify connectivity, reporting statuses on the dashboard.
- **Auto-Boot & Remote DB Launch**: Supports triggering cross-platform database startups (Docker, macOS Brew, Linux systemd, Windows Services) both automatically at boot and on-demand from the web UI.
- **OpenMFE Web Dashboard**: Serves a dynamic loader script with integrated cache-busting and client-side reload headers.
- **Single-Instance Locking**: Obtains a secure file lock on `.watchdog-agent.lock` using Unix `Flock` or Windows exclusive `CreateFile` calls.

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

---

*Version: v0.0.1 (See main.go)*
