# AGENTS.md: watchdog-agent

## Service Mission & Architecture Role
`watchdog-agent` is the local node supervisor, process orchestrator, health monitor, and configuration symlink healer for the Bastien-Antigravity fleet. It ensures that required daemons (such as `log-server`, `config-server`, `nats-server`, `web-interface`, `notif-server`, and trading/analysis services) remain operational, restarts failed processes with exponential backoff, exports Prometheus metrics, and automatically heals broken or missing `standalone.yaml` symlinks across the ecosystem.

- **Exposed Port**: `9095` (REST Management API & OpenMFE)
- **Key Modules**:
  - `src/supervisor`: Process launcher, health checks, PID tracking, graceful shutdown
  - `src/config/heal.go`: Auto-repair engine for all 35 base ecosystem `standalone.yaml` symlinks
- **Configuration Link**: `standalone.yaml -> ../docker-deployment/modes/local/config/native.yaml`

## Key Build & Test Commands
```bash
# Build binary
go build -o bin/watchdog-agent main.go

# Run tests
go test -v ./...

# Run service
./bin/watchdog-agent
```

## AI Development & Integration Guidelines
1. **No User-Home Scans**: When resolving executables (like `nats-server`), use standard `$PATH` lookups or explicit project paths. Never scan `os.UserHomeDir()` or `~/.local/bin`.
2. **Canonical Service Ports**: Respect canonical port allocations (e.g. `web-interface` is `5000`, `log-server` is `9020`, `config-server` is `8090`).
3. **Symlink Self-Healing**: When adding new microservices to the base ecosystem, register their symlink path in `src/config/heal.go`.
4. **Header Ritual**: All Go source files MUST contain the Triple-Block header (`ESSENTIAL PROCESS`, `DATA FLOW`, `KEY PARAMETERS`).
5. **Section Dividers**: Use `// -----------------------------------------------------------------------------` between exported methods and major sections.
6. **L1 Resilience, Connectivity & Infinite Retry Contract**:
   - OpenMFE registration to `web-interface` must run in a non-blocking background goroutine with infinite retries (`MaxRetries: -1`), exponential backoff, and dynamic address resolution via `cfg.GetListenAddr("web_interface")`.
   - `OpenMFE & Exchange Contracts`: Exposes `GET /api/v1/contracts` (REST, NATS control subjects, TimescaleDB `fleet_error_logs` SQL DDL, and client probes), publishes `ServiceSpec.Contracts`, and mounts dynamic tabs in `#watchdog-tabs-bar` (`src/rest/mfe.js`).
   - NATS control plane connection must retry with exponential backoff on startup until the broker is online; it must never permanently abort on initial connection failure.
7. **Dynamic NATS Process Supervision & Zero-Conf Launch**:
   - `nats-server` process launch is driven entirely by dynamic CLI arguments constructed from `capabilities.nats` in configuration (`-js`, `-sd <store_dir>`, `-p <port>`, `-a <ip>`, `-m <http_port>`, `-n <server_name>`).
   - If `tls.enabled: true` or non-loopback IP with certificate assets is configured, the supervisor automatically injects `--tls`, `--tlscert`, `--tlskey`, `--tlsverify`, and `--tlscacert`.
   - Never rely on static or hardcoded `nats-server.conf` files or directories.
8. **Live Config Address Resolution & Atomic Duplicate Process Eviction (`svc.ResolveAddr()` & `svc.ActiveCmd`)**:
   - Supervised services resolve their current `ip` and `port` dynamically via `svc.ResolveAddr()`, reading `AppConfig`'s lock-free `LiveConfig` atomic pointer (`service_addresses` broadcast registry or live capability overrides) with fallback to static `Capabilities`.
   - `Service.ActiveCmd` (`atomic.Pointer[exec.Cmd]`) atomically swaps the active process handle when a new instance is spawned (`old := svc.ActiveCmd.Swap(cmd)`), immediately terminating any older duplicate instance via `utils.KillProcessTree(old)` and using `svc.ActiveCmd.CompareAndSwap(cmd, nil)` on exit so an exiting process never clears a newer replacement's handle.
