---
microservice: watchdog-agent
type: features-behavior
status: active
language: go
tags:
- '#ai/ignore'
- '#service/watchdog-agent'
- '#type/features-behavior'
- '#state/active'
---

# 🕹️ Watchdog Agent — Features & Behavior

Detailed reference of process supervision and recovery behavior of the `watchdog-agent`.

## Core Supervisory Loop

For each managed microservice, the watchdog performs the following sequence in a dedicated supervisor goroutine:

1. **Dependency Resolution**: Loops and sleeps until all sibling services listed in `Deps` are listening on their configured TCP addresses.
2. **Orphan Port Pruning**: If the service's target port is already occupied, the agent checks if the occupant is an ecosystem/fleet process. If so, it forcefully terminates the occupant and all of its subprocesses (using Unix process group signals or Windows `taskkill /F /T`) to guarantee clean port binding.
3. **Execution**: Spawns the subprocess, assigning platform-appropriate group descriptors (e.g., `Setpgid` on Unix or `CREATE_NEW_PROCESS_GROUP` on Windows) and injecting environment flags.
4. **Log Redirection**: Continuously reads stdout/stderr streams from pipes and prints them to console output with color-coded service prefixes.
5. **Crash Recovery**: If the child exits, the agent prints the exit status, waits 3 seconds, and restarts the lifecycle loop.

## Telemetry & Dashboard Control

The watchdog provides central status checking for non-supervised external components:

- **Verification**: Continuously tests connectivity of **TimescaleDB** (Postgres) and the **RAG MCP Server** via TCP pings (500ms timeout) using the resolved capability addresses from configuration.
- **OpenMFE dashboard**: Renders real-time telemetry indicators. External dependencies are represented in the main fleet table as `External` components.
- **Database Launch Controls**: If `timescale-db` is offline, the dashboard displays a **"Start DB"** button which triggers a POST request to `/api/v1/postgres/start`. This initiates a cross-platform database boot sequence in the background, supporting Docker (`docker start timescale-db`), macOS Homebrew (`brew services start`), Linux `systemctl`, and Windows `net start` services.
- **Cache Control**: Served script assets use active cache-busting headers combined with browser-side URL timestamp loading to prevent browser cache issues.
