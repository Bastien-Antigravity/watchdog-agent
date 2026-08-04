---
microservice: watchdog-agent
type: general-misc
status: active
language: go
tags:
- '#ai/ignore'
- '#service/watchdog-agent'
- '#type/general-misc'
- '#state/active'
---

# 📝 Watchdog Agent — General & Miscellaneous

Supplementary notes and common operational commands.

## Graceful Cleanups
When the watchdog receives an interrupt signal (`SIGINT` or `SIGTERM`), it automatically terminates all spawned subprocesses:
- **On Unix-like systems (macOS, Linux):** It resolves the Process Group ID (PGID) and signals the entire process group with `SIGTERM` followed by `SIGKILL` to clean up child processes.
- **On Windows systems:** It executes the native `taskkill /F /T /PID` command to forcefully prune the entire process tree under the target PID.
This ensures no orphan processes (e.g. running Python or Rust instances) are left behind on shutdown.

## Environment Exporters
During startup, the agent dynamically appends environment configurations (including FFI bridge references `LIBUNILOG_PATH` and `LIBSAFESOCKET_PATH`) to all child process environments, eliminating the need to set them manually in shell configurations.

## Single-Instance Locks
To prevent parallel execution and port binding conflicts:
- **Unix:** Obtains an exclusive flock (`syscall.LOCK_EX | syscall.LOCK_NB`) on `.watchdog-agent.lock`.
- **Windows:** Opens `.watchdog-agent.lock` with exclusive sharing flags (`CreateFile` with `0` share mode), blocking any other instance from opening the lock handle.
