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
