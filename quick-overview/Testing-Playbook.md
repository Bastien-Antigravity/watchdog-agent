---
microservice: watchdog-agent
type: testing-playbook
status: active
language: go
tags:
- '#ai/ignore'
- '#service/watchdog-agent'
- '#type/testing-playbook'
- '#state/active'
---

# 🧪 Watchdog Agent — Testing Playbook

Manual verification instructions for `watchdog-agent` process locking and sync capability operations.

## Manual Testing Scenarios

### 1. Process Lock Exclusivity Check
- Run the supervisor in one terminal:
  ```bash
  go run main.go
  ```
- Attempt to launch it in a second terminal:
  ```bash
  go run main.go
  ```
- **Expected Result**: The second instance logs:
  `Another instance of watchdog-agent is already running (failed to acquire lock). Exiting.`
  and exits with code 1.

### 2. Dependencies Block Check
- Terminate any running `log-server` processes.
- Run a service that depends on it (e.g. `config-server`) directly or check the watchdog startup logs.
- **Expected Result**: The watchdog logs:
  `Waiting for dependency 'log-server' on 127.0.0.1:9020...`
  and pauses execution until the port becomes active.

### 3. Fleet Error Log Shipper Verification
- **Automated Unit Tests**:
  ```bash
  go test -v ./src/shipper/...
  ```
  **Expected Result**: All 7 tests pass:
  - `TestParseLogLine_FixedFormat`
  - `TestParseLogLine_CriticalSeverity`
  - `TestParseLogLine_IgnoreInfoAndWarning`
  - `TestParseLogLine_FallbackRegex`
  - `TestShipper_DiscoverFilesAndCheckpoint`
  - `TestShipper_RotationDetection`
  - `TestShipper_OfflineDB_OffsetsFreeze`

- **Manual Insertion & Checkpoint Test**:
  1. Append an error line to any service log (e.g. `echo '2026-11-09T14:00:00.000000000Z host service ERROR file.go func 1 Failed to load' >> log-server/logs/log-server.log`).
  2. Run `watchdog-agent` or let background loop trigger.
  3. Inspect `.shipper_offsets.json` to verify the byte offset advances.
  4. Query TimescaleDB:
     ```sql
     SELECT timestamp, service_name, level, message FROM fleet_error_logs ORDER BY timestamp DESC LIMIT 5;
     ```
     **Expected Result**: The logged error appears in the hypertable with parsed metadata.
