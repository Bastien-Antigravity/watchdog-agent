---
microservice: watchdog-agent
type: project-dna
status: active
tags:
- '#service/watchdog-agent'
- '#type/project-dna'
- '#state/active'
---

# 🧬 watchdog-agent — AI Project DNA

System intent and architectural boundaries for the `watchdog-agent`.

## 🎯 System Intent
The agent acts as a supervisor that orchestrates the execution of:
- `log-server` (Rust)
- `config-server` (Go)
- `notif-server` (Go)
- `tele-remote` (Go)
- `rag-engine` (Python, RAG MCP)
- `rag-dashboard` (Python)
- `web-interface` (Go)
- `start-squad` (Go)

It also acts as a telemetry broker that monitors:
- `timescale-db` (External TimescaleDB/Postgres database)
- `rag-mcp` (External RAG MCP server interface)

It maintains execution order using topological cycle-detection validations, syncs environment setups, and exposes control planes via HTTP REST.

## 🚫 Boundaries
- **No Configuration Mutation**: The watchdog does not edit configuration files directly. It validates target listening ports and retries checking them, warning on conflict rather than mutating properties.
- **Cross-Platform Lock Exclusivity**: It uses `.watchdog-agent.lock` exclusively, implementing platform-specific locks (Unix `Flock` and Windows exclusive share handles) to prevent multi-instance collisions.
- **External Dependency Limits**: The watchdog cannot supervise the lifetime of `timescale-db` or `rag-mcp` as standard child processes, but it offers cross-platform launch hooks (Docker, Brew, systemd, Windows Services) to start them.
- **REST Restart Operations**: Virtual services (`timescale-db`, `rag-mcp`) must be excluded from automated service restarts in client tooling.
