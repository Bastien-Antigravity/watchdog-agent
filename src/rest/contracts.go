package rest

// =============================================================================
// ESSENTIAL PROCESS:
// Constructs exhaustive, microservice-authored Exchange Contracts for watchdog-agent
// covering its REST supervision API, NATS heartbeat & incident bus, TimescaleDB
// error log shipper SQL DDL, and ecosystem client/probe integrations.
//
// DATA FLOW:
// 1. Resolves live NATS, TimescaleDB, Tele-Remote, RAG MCP, and REST addresses
//    from AppConfig + Controller runtime status.
// 2. Packages contracts using FleetErrorLogsSchemaSQL directly from shipper.db.
//
// KEY PARAMETERS:
// - appConfig: Optional ecosystem AppConfig resolving runtime ports.
// - supervisedNames: Live list of supervised fleet service names.
// =============================================================================

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_messaging "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/messaging"
	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
	toolbox_utils "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/utils"
	"github.com/Bastien-Antigravity/watchdog-agent/src/server"
	"github.com/Bastien-Antigravity/watchdog-agent/src/shipper"
)

// -----------------------------------------------------------------------------

// BuildExchangeContracts constructs the exhaustive contract catalog from watchdog-agent sources.
func BuildExchangeContracts(appConfig *toolbox_config.AppConfig, supervisedNames []string) []toolbox_spec.ExchangeContract {
	var restAddr, natsAddr, pgAddr, ragMcpAddr, teleAddr string
	natsPrefix := "bastien-antigravity"
	brainID := "obsidian-brain"

	if appConfig != nil {
		restAddr, _ = appConfig.GetRESTAddr("watchdog_agent")
		natsAddr, _ = appConfig.GetListenAddr("nats")
		pgAddr, _ = appConfig.GetListenAddr("timescale_db")
		teleAddr, _ = appConfig.GetGRPCListenAddr("tele_remote")
		if teleAddr == "" {
			teleAddr, _ = appConfig.GetListenAddr("tele_remote")
		}
		var natsCfg toolbox_messaging.NatsConfig
		if err := appConfig.GetCapability("nats", &natsCfg); err == nil {
			if natsCfg.SubjectPrefix != "" {
				natsPrefix = natsCfg.SubjectPrefix
			}
		}
		var ragCap struct {
			MCP struct {
				IP   string `json:"ip"`
				Port string `json:"port"`
			} `json:"mcp"`
		}
		if err := appConfig.GetCapability("rag_engine", &ragCap); err == nil && ragCap.MCP.IP != "" && ragCap.MCP.Port != "" {
			ragMcpAddr = net.JoinHostPort(ragCap.MCP.IP, ragCap.MCP.Port)
		}
		brainID = shipper.ResolveBrainID(appConfig)
	}

	hostname := toolbox_utils.GetHostname()
	nodeStatusSubject := fmt.Sprintf("%s.watchdog.node.%s.status", natsPrefix, hostname)
	incidentSubject := shipper.ResolveIncidentSubject(natsPrefix, brainID)

	var supervisedPeerDesc string
	if len(supervisedNames) > 0 {
		supervisedPeerDesc = fmt.Sprintf("Live supervised processes: %s", strings.Join(supervisedNames, ", "))
	} else {
		supervisedPeerDesc = "Supervises nats-server, log-server, config-server, notif-server, tele-remote, web-interface, rag_engine"
	}

	return []toolbox_spec.ExchangeContract{
		{
			ID:        "rest",
			Type:      "HTTP/REST API",
			TabLabel:  "HTTP/REST API",
			Icon:      "fa-globe",
			Direction: "inbound",
			Transport: "HTTP/1.1 + CORS JSON",
			Address:   toolbox_spec.DisplayAddress(restAddr),
			Port:      toolbox_spec.ResolveContractPort(restAddr),
			Framing:   "HTTP JSON request/response with no-cache MFE delivery",
			Auth:      "Internal mesh REST management",
			Summary:   "Exposes fleet process status, single/all service restart commands, PostgreSQL launch triggers, and <watchdog-agent-mfe>.",
			Source:    "watchdog-agent/src/rest/rest_handler.go",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "web_interface", Role: "consumer", Description: "Loads <watchdog-agent-mfe> and triggers restart/health actions"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "Status", Kind: "GET", Target: "/api/v1/status", Output: "{ healthy, status, version, uptime_seconds, services[], postgres_connected, rag_mcp_connected }", Description: "Live supervised process roster & infra connectivity"},
				{Name: "RestartService", Kind: "POST", Target: "/api/v1/watchdog/restart", Input: "{ name: string }", Output: "{ success, message, timestamp }", Description: "Signals supervisor to restart a supervised service"},
				{Name: "RestartAll", Kind: "POST", Target: "/api/v1/watchdog/restart_all", Output: "{ success, message, timestamp }", Description: "Orchestrates rolling restart across supervised services"},
				{Name: "PauseService", Kind: "POST", Target: "/api/v1/watchdog/pause", Input: "{ name: string }", Output: "{ success, message, timestamp }", Description: "Suspends auto-restart supervision and terminates active process"},
				{Name: "ResumeService", Kind: "POST", Target: "/api/v1/watchdog/resume", Input: "{ name: string }", Output: "{ success, message, timestamp }", Description: "Resumes auto-restart supervision and clears crash backoff"},
				{Name: "StartPostgres", Kind: "POST", Target: "/api/v1/postgres/start", Output: "{ success, message, timestamp }", Description: "Triggers async TimescaleDB container/daemon launch"},
				{Name: "Contracts", Kind: "GET", Target: "/api/v1/contracts", Output: "ContractsEnvelope JSON", Description: "Self-describing microservice exchange contracts"},
				{Name: "MFEScript", Kind: "GET", Target: "/static/mfe.js", Output: "application/javascript", Description: "Embedded OpenMFE <watchdog-agent-mfe> with no-store headers"},
			},
			SchemaFormat: "http",
			Schema: strings.Join([]string{
				"GET  /api/v1/status",
				"GET  /api/v1/contracts",
				"POST /api/v1/watchdog/restart     body: {\"name\": \"config-server\"}",
				"POST /api/v1/watchdog/restart_all",
				"POST /api/v1/watchdog/pause       body: {\"name\": \"tele-remote\"}",
				"POST /api/v1/watchdog/resume      body: {\"name\": \"tele-remote\"}",
				"POST /api/v1/postgres/start",
				"GET  /static/mfe.js                 (alias: /static/js/mfe-loader.js)",
			}, "\n"),
		},
		{
			ID:        "nats",
			Type:      "NATS Pub/Sub",
			TabLabel:  "NATS Control Plane",
			Icon:      "fa-paper-plane",
			Direction: "outbound",
			Transport: "NATS Core TCP Pub/Sub",
			Address:   toolbox_spec.DisplayAddress(natsAddr),
			Port:      toolbox_spec.ResolveContractPort(natsAddr),
			Framing:   "JSON payloads over NATS subjects (5s heartbeat + 60s debounced incidents)",
			Auth:      "Capability nats credentials",
			Summary:   "Publishes periodic node process heartbeats and real-time fleet error incidents parsed by the log shipper.",
			Source:    "watchdog-agent/src/control/control_plane.go + src/shipper/incident.go",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "nats", Role: "broker", Description: "Central message bus carrying watchdog heartbeats & incident triggers"},
				{Service: "agent_squad", Role: "consumer", Description: "Subscribes to fleet incidents for automated AI triage"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "NodeHeartbeat", Kind: "PUB", Target: nodeStatusSubject, Input: "NodeStatus { node, timestamp, services: map[string]{ status, ip, port, pid } }", Description: "5-second node & supervised process heartbeat"},
				{Name: "FleetIncident", Kind: "PUB", Target: incidentSubject, Input: "IncidentEvent { incident_id, service_name, level_str, filename, message, occurrence_count }", Description: "Throttled error/critical log incident event (60s cooldown)"},
			},
			SchemaFormat: "json",
			Schema: fmt.Sprintf(`{
  "heartbeat_subject": %q,
  "heartbeat_interval_sec": 5,
  "heartbeat_payload": {
    "node": %q,
    "timestamp": 1730000000,
    "services": {
      "<name>": { "status": "RUNNING | STOPPED", "ip": "<host>", "port": "<port>", "pid": 12345 }
    }
  },
  "incident_subject": %q,
  "incident_cooldown_sec": 60,
  "incident_payload": {
    "incident_id": "string",
    "brain_id": "string",
    "event_hash": "sha256-hex",
    "timestamp": "RFC3339",
    "service_name": "string",
    "hostname": "string",
    "level": 10,
    "level_str": "ERROR | CRITICAL",
    "filename": "string",
    "function_name": "string",
    "line_number": "string",
    "message": "string",
    "stack_trace": "string (optional)",
    "occurrence_count": 1
  }
}`, nodeStatusSubject, hostname, incidentSubject),
		},
		{
			ID:        "timescaledb-sql",
			Type:      "TimescaleDB SQL Sink",
			TabLabel:  "TimescaleDB SQL",
			Icon:      "fa-database",
			Direction: "outbound",
			Transport: "PostgreSQL Wire Protocol v3 (lib/pq)",
			Address:   toolbox_spec.DisplayAddress(pgAddr),
			Port:      toolbox_spec.ResolveContractPort(pgAddr),
			Framing:   "Atomic SQL transactions inserting parsed ERROR/CRITICAL log batches into obsidiandb",
			Auth:      "Capability timescale_db user + decrypted password + optional SSL",
			Summary:   "Ensures watchdog_agent.fleet_error_logs hypertable schema in obsidiandb and idempotently ships checkpointed fleet errors.",
			Source:    "watchdog-agent/src/shipper/db.go (FleetErrorLogsSchemaSQL)",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "timescale_db", Role: "server", Description: "Persists brain-partitioned fleet error logs into 1-day hypertable chunks (30d retention)"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "EnsureSchema", Kind: "DDL", Target: "watchdog_agent.fleet_error_logs", Input: "CREATE SCHEMA + TABLE + create_hypertable(1 day) + retention(30 days)", Description: "Verifies schema, hypertable, & indexes at startup"},
				{Name: "InsertBatch", Kind: "SQL", Target: "INSERT INTO watchdog_agent.fleet_error_logs (...13 cols...) ON CONFLICT DO NOTHING", Input: "ParsedErrorLog[] transaction batch", Description: "Idempotent error log persistence prior to cursor checkpoint"},
			},
			SchemaFormat: "sql",
			Schema:       shipper.FleetErrorLogsSchemaSQL,
		},
		{
			ID:        "ecosystem-clients",
			Type:      "Supervisor & Client Integrations",
			TabLabel:  "Probes & Clients",
			Icon:      "fa-plug",
			Direction: "outbound",
			Transport: "TCP Probes + SafeSocket + gRPC Stream",
			Address:   fmt.Sprintf("pg=%s mcp=%s tele=%s", toolbox_spec.DisplayAddress(pgAddr), toolbox_spec.DisplayAddress(ragMcpAddr), toolbox_spec.DisplayAddress(teleAddr)),
			Framing:   "TCP port dial probes, TeleRemote gRPC bidi stream, supervised process IPC",
			Auth:      "Local host orchestration / capability addresses",
			Summary:   "Orchestrates child service lifecycles, exposes interactive Tele-Remote supervision menus, and probes database & MCP sockets.",
			Source:    "watchdog-agent/src/supervisor/registry.go + src/telegram/manager.go",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "Supervised Fleet", Role: "supervisor", Description: supervisedPeerDesc},
				{Service: "tele_remote", Role: "server", Description: "Interactive Telegram restart & fleet status menu client"},
				{Service: "rag_mcp", Role: "probed_target", Description: "TCP connectivity health check from Watchdog controller"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "TeleRemoteConnect", Kind: "BIDI_STREAM", Target: "teleremote.TeleRemoteService/Connect", Description: "Publishes live service status & restart controls to Telegram"},
				{Name: "PostgresProbe", Kind: "TCP", Target: toolbox_spec.DisplayAddress(pgAddr), Output: "postgres_connected bool", Description: "500ms TCP dial check for TimescaleDB availability"},
				{Name: "RAGMCPProbe", Kind: "TCP", Target: toolbox_spec.DisplayAddress(ragMcpAddr), Output: "rag_mcp_connected bool", Description: "500ms TCP dial check for FastMCP SSE availability"},
			},
			SchemaFormat: "wire",
			Schema:       "Supervised Fleet Orchestration:\n- Pre-flight builds / Docker CLI startup for infrastructure containers\n- Config symlink healing & offset checkpointing (.shipper_offsets.json)\n- Live health checks reported on /api/v1/status",
		},
	}
}

// -----------------------------------------------------------------------------

func (h *RESTHandler) handleContracts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var svcNames []string
	version := server.ServerVersion
	if statusInfo, err := h.control.GetStatus(r.Context()); err == nil {
		if statusInfo.Version != "" {
			version = statusInfo.Version
		}
		for _, svc := range statusInfo.Services {
			svcNames = append(svcNames, svc.Name)
		}
	}

	contracts := BuildExchangeContracts(h.appConfig, svcNames)
	h.sendJSON(w, toolbox_spec.NewContractsEnvelope("watchdog_agent", version, "watchdog-agent (live supervisor + shipper DDL + NATS config)", contracts))
}
