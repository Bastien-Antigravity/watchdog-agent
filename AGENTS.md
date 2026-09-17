# AGENTS.md: watchdog-agent

## Service Mission & Architecture Role
`watchdog-agent` is the local node supervisor, process orchestrator, health monitor, and configuration symlink healer for the Bastien-Antigravity fleet. It ensures that required daemons (such as `log-server`, `config-server`, `nats-server`, `web-interface`, `notif-server`, and trading/analysis services) remain operational, restarts failed processes with exponential backoff, exports Prometheus metrics, and automatically heals broken or missing `standalone.yaml` symlinks across the ecosystem.

- **Exposed Port**: `8002` (REST API & `/metrics`)
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
