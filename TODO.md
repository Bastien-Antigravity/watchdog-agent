---
microservice: watchdog-agent
type: task
status: active
tags:
- '#service/watchdog-agent'
- '#type/task'
- '#state/active'
- '#zone/3-fleet'
---

# TODO List: watchdog-agent

## 🚀 Autonomous Fleet Error Log Shipper & Auto-Repair Subsystem

### 📊 Global Synthesis of Current Functionality
The `watchdog-agent` embeds an autonomous error log shipping and incident notification pipeline designed for zero-loss operational observability across native and containerized stacks:
1. **Autonomous Multi-File Discovery**: Automatically discovers `*/logs/*.log` natively in repository trees (filtering forwarded non-`log_server` rows out of `_main.log` to avoid duplication) or `/var/log/fleet/<service>/*.log` in containerized shared Docker volumes (`fleet-logs`).
2. **Dual 8-Column & Cloud JSON Parser**: Fast-path parser supporting standard fixed-width 8-column text (`timestamp`, `host`, `logger`, `level`, `file`, `func`, `line`, `message` with regex fallback) and `flexible-logger` `JSONSerializer` cloud logs, sanitizing non-UTF8 / NUL bytes and filtering strictly for `ERROR` (`10`) and `CRITICAL` (`11`).
3. **Resilient Offset Checkpointing**: Atomically tracks persistent byte offsets and filesystem inode identifiers in `.shipper_offsets.json` with file truncation, inode rotation detection, and stale missing-file pruning.
4. **Option B TimescaleDB Ingestion & Startup DDL**: Automatic schema verification (`EnsureSchema`) creating schema `watchdog_agent`, hypertable `watchdog_agent.fleet_error_logs` (with `brain_id` + `event_hash` unique constraint and `db_browser_reader` conditional grants) inside `obsidiandb`.
5. **Database Downtime Resilience**: Transactions with offset freeze — offsets remain untouched if DB is unreachable, backed by `conn_manager.NewCriticalStrategy` backoff (`ensureConnected`).
6. **Modular Operational Switches**: `--mode=all|supervisor|shipper|telemetry`, with Docker decoupling (`supervise: false` under `DOCKER_ENV=true` / `log-shipper`).
7. **Real-Time Brain-Partitioned Squad Incident Forwarding**: Asynchronous dispatch over NATS (`<prefix>[.<brain>].fleet.incident`) via `IncidentThrottler` (60s cooldown + occurrence counters) consumed in real time by `08-Squad-Agent` `AutoRepairCoordinator`.

---

### 🛡️ Deepening Roadmap: Hardening, Behavior, Architecture & Testing

- [ ] **1. Hardening & Fault Resilience**:
  - [x] **State Pruning & Compaction**: Implemented `CheckpointManager.PruneMissingFiles` to purge deleted/rotated log file keys from `.shipper_offsets.json`.
  - [x] **Poison-Pill & Non-UTF8 Sanitization**: Implemented `sanitizeLogText` (`strings.ToValidUTF8` + NUL byte removal) before database batch insertion.
  - [x] **Database Connection Pool & Backoff Hardening**: Implemented `conn_manager.NewCriticalStrategy` exponential backoff in `ensureConnected()` and strict AppConfig fail-fast resolution (`ResolveDBConfigWithAppConfig`).
  - [ ] **Database Statement Timeout & Filesystem Edge Cases**: Add PostgreSQL `statement_timeout` configuration, read-only/permission-denied handling polish, and mid-read rotation race stress tests.

- [ ] **2. Behavior & Functionality Definition**:
  - [x] **Canonical Severity & Cloud JSON Support**: Aligned Go (`parser.go`), Python (`unilog`), Rust, C++, and VBA on `flexible-logger`'s 12 canonical levels (`ERROR=10`, `CRITICAL=11`) and added `parseJSONLogLine` for `NewCloudLogger`.
  - [ ] **Incident Burst & Anomaly Metrics**: Track and compute moving-average error frequency per service, tagging incidents as `BURST` when error frequency exceeds defined thresholds (e.g. >10 errors/minute).
  - [ ] **TimescaleDB Data Retention Policy**: Add configurable automated chunk drops (e.g. `add_retention_policy('watchdog_agent.fleet_error_logs', INTERVAL '30 days')`) to prevent unbounded disk growth.
  - [x] **Incident Acknowledgment & State Sync**: Implemented `08-Squad-Agent` `AutoRepairCoordinator` (`watchdog_agent.fleet_incidents`, signature coalescing, on-the-fly pause/enable toggle, REST & Telegram approval/reject/resolve).

- [ ] **3. Architecture & Multi-Node Topology**:
  - [ ] **Distributed Multi-Node TLS Security**: Define and document mutual TLS (mTLS) configuration for `log-shipper` containers on remote machines communicating with centralized TimescaleDB and NATS clusters.
  - [ ] **Formal BDD Specification**: Author `FEAT-010-Autonomous-Error-Log-Shipping.md` in `obsidian-brain/02-Business-BDD/02-Behavior-Specs/watchdog-agent/` defining Gherkin scenarios for tailing, checkpointing, rotation, and throttled incident dispatch.
  - [ ] **Dashboard Integration in OpenMFE**: Add an "Error Logs & Auto-Repair" tab to the embedded OpenMFE dashboard (`mfe.js`) displaying recent hypertable rows and real-time SRE agent remediation events.

- [ ] **4. Testing & Verification Matrix**:
  - [ ] **High-Throughput Micro-Benchmarks**: Add Go benchmark suite (`BenchmarkParseLogLine`, `BenchmarkIncidentThrottler`) ensuring zero heap allocations on the hot path under 100,000 lines/sec.
  - [ ] **Chaos & Network Partition Integration Test**: Implement automated chaos test injecting database connection drops mid-batch alongside live TimescaleDB (`offline DB` cursor freeze covered by `TestShipper_OfflineDB_OffsetsFreeze`).
  - [ ] **End-to-End Container Test**: Add an end-to-end integration test spinning up `timescale-db`, generating multi-service log files, and asserting correct row insertion into `watchdog_agent.fleet_error_logs`.
  - [x] **Auto-Repair Unit & Worktree Drill**: Implemented comprehensive `watchdog-agent` shipper/incident/config unit suite (`go test ./...`) and `08-Squad-Agent/tests/test_autorepair.py` (NATS publish/subscribe, signature coalescing, on-the-fly toggle, isolated `/tmp` worktree verification, 3-strike circuit breaker, and REST routes).

---

## 🛠️ General Watchdog Maintenance & Core Supervision
- [x] Process supervision and health checking
- [x] Configuration symlink auto-healing engine (`heal.go`)
- [x] Operational mode flags (`--mode`, `--supervise`, `--shipper`, `--rest`)
- [ ] Continuous compliance and port invariant verification
