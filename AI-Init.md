---
microservice: watchdog-agent
type: ai-init
status: active
tags:
- '#service/watchdog-agent'
- '#type/ai-init'
- '#state/active'
---

# 🤖 watchdog-agent — AI Onboarding Instructions

Guidelines for AI agents interacting with the `watchdog-agent` repository.

## 🛠️ Environment Abstractions
To run and inspect the supervisor:
1. Ensure the root-level virtual environment `.venv` is loaded.
2. The Go execution path must be pointing to Go 1.22+.
3. Build configurations use the `standalone.yaml` configuration profiles.

## 🔍 Code Review Checkpoints
- **Process Supervision Loop**: Look at `MonitorAndSupervise` in [supervisor.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/supervisor/supervisor.go#L119) for the child process supervision lifecycle loop.
- **Topological Validations**: Look at `ValidateRegistry` in [registry.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/supervisor/registry.go#L179) to inspect dependency topological checks (deadlocks/missing siblings detection).
- **Group Process Termination**: Look at [sys_unix.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/utils/sys_unix.go) and [sys_windows.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/utils/sys_windows.go) to see cross-platform group process management.
- **Database Launcher**: Look at [postgres_launcher.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/supervisor/postgres_launcher.go) to inspect the cross-platform TimescaleDB startup wrapper.
- **Single-Instance Locks**: Inspect platform locks in [lock_unix.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/utils/lock_unix.go) and [lock_windows.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/utils/lock_windows.go).
- **Web Component (OpenMFE)**: Look at [mfe.js](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/rest/mfe.js) to view the dashboard user interface logic.
- **REST Endpoints**: Look at [rest_handler.go](file:///Users/imac/Desktop/Bastien-Antigravity/watchdog-agent/src/rest/rest_handler.go) for REST routes registration and cache-control mappings.
