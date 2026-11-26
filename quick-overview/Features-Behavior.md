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
- '#zone/3-fleet'
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

## Autonomous Fleet Error Log Shipper

The watchdog agent embeds an autonomous background error-log shipping worker (`src/shipper/`) that acts as a reliable pipeline connecting file-based service logs to TimescaleDB:

1. **Multi-Environment Discovery**:
   - In Docker containerized stacks, microservices mount a shared volume (`fleet-logs:/var/log/fleet/<service-name>`). A dedicated `log-shipper` container running the watchdog agent mounts `/var/log/fleet` in read-only mode.
   - In native local development, the shipper traverses the repository directory tree scanning for `*/logs/*.log` files while ignoring `.git`, `node_modules`, `target`, and `vendor`.
2. **Zero-Allocation 8-Column Log Parsing**:
   - Parses the standard 8-column log format: `timestamp(33) host(12) logger(22) level(10) file(20) func(25) line(6) msg`.
   - Uses zero-heap-allocation byte column slicing, with regex fallback for non-standard lines.
3. **Severity Filtering**:
   - Selects only entries with severity `level >= ERROR` per `flexible-logger/src/models/level.go` (`ERROR` = `10`, `CRITICAL` = `11`), filtering out `DEBUG` (`1`) through `WARNING` (`9`) logs.
4. **Resilient Offset Tracking (`.shipper_offsets.json`)**:
   - Tracks persistent byte offsets and filesystem inode identifiers for each monitored log file.
   - Saves checkpoint files atomically using temporary files and OS renames (`.shipper_offsets.json.tmp` -> `.shipper_offsets.json`).
5. **Log Rotation & Truncation Handling**:
   - Compares the current file size and OS inode against the stored checkpoint.
   - If the file has shrunk or the inode changed (rotation), the offset resets to 0 immediately to prevent skipping new error messages.
6. **Zero-Loss Database Downtime Recovery**:
   - Log cursor offsets are only updated in memory and committed to disk **after** the TimescaleDB transaction `tx.Commit()` succeeds.
   - If TimescaleDB is offline or unreachable, the shipper logs a retry warning, backs off for 5 seconds, and freezes checkpoint offsets. Once the database is reachable, all uncommitted error logs are flushed and committed without loss.
7. **Real-Time Incident Forwarding & Auto-Repair Triage**:
   - In parallel with batch database ingestion, the shipper evaluates an `IncidentThrottler` (60s cooldown per `service:file:line` signature).
   - Qualifying incidents are dispatched over the ecosystem event bus (`bastien-antigravity.fleet.incident`), triggering real-time triage by the Squad Agent and **Role 15: Fleet Healer & Auto-Repair Sentinel** ([[Prompt-Fleet-AutoRepair.md]]).

## Distributed Operation & Orchestrator Decoupling

In distributed multi-node topologies across different physical machines or cloud instances, service execution models differ between **Native Host Mode** and **Docker Container Mode**:

1. **Native Host Distributed Mode**:
   - Each machine runs an instance of `watchdog-agent`.
   - The agent inspects `utils.IsLocal(svc.IP, localIPs)` against the machine's local interfaces.
   - It compiles, executes, monitors, and restarts **only** the microservices assigned to its specific IP address, while waiting for remote dependencies across TCP.
   - If a local service exits unexpectedly, `watchdog-agent` detects the process termination and restarts it after a 3-second delay.

2. **Docker Container Distributed Mode**:
   - In containerized deployments, **Docker daemon** is the authoritative process supervisor:
     - Container restart policies (`restart: unless-stopped`) govern automatic restarts upon crash or error exit.
     - Docker isolates processes and manages virtual network routing.
   - `watchdog-agent` runs in **Shipper Mode** (`--mode=shipper` or auto-detected via `DOCKER_ENV=true`):
     - `enableSupervise` is set to `false`: process compilation, process group spawning, and port pruning are completely bypassed.
     - `enableShipper` is active: it mounts the local node's `fleet-logs` shared volume, tailing error logs and shipping them to the centralized TimescaleDB hypertable.
   - This prevents race conditions and resource conflicts between the native OS supervisor and the Docker container engine.
