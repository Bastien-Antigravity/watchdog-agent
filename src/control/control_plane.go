package control

// =============================================================================
// ESSENTIAL PROCESS:
// NATS control plane client and telemetry publisher for watchdog-agent.
// Connects asynchronously with exponential backoff, publishes periodic node
// heartbeats, and exposes lock-free Publish for real-time fleet incidents.
//
// DATA FLOW:
// 1. Resolves NATS capability credentials once from AppConfig on startup.
// 2. Connects to NATS broker with backoff and atomically stores *nats.Conn.
// 3. Optionally runs 5s heartbeat publisher to '<prefix>.watchdog.node.<host>.status'.
// 4. Provides non-blocking Publish() for shipper incident event forwarding.
//
// KEY PARAMETERS:
// - cfg: Ecosystem AppConfig providing dynamic NATS capability configuration.
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_conn "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/conn_manager"
	toolbox_logger "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/logger"
	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/messaging"
	toolbox_utils "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/utils"

	"github.com/nats-io/nats.go"
)

// -----------------------------------------------------------------------------

// BusClient manages the shared NATS connection for heartbeats and fleet incidents.
type BusClient struct {
	natsCfg       messaging.NatsConfig
	subjectPrefix string
	hostname      string
	logger        toolbox_logger.Logger
	conn          atomic.Pointer[nats.Conn]
}

// -----------------------------------------------------------------------------

// NewBusClient resolves NATS configuration from validated AppConfig on cold startup.
func NewBusClient(cfg *toolbox_config.AppConfig, l toolbox_logger.Logger) (*BusClient, error) {
	var natsCfg messaging.NatsConfig
	if err := cfg.GetCapability("nats", &natsCfg); err != nil {
		return nil, fmt.Errorf("failed to resolve nats capability: %w", err)
	}
	if natsCfg.ClientID == "" {
		natsCfg.ClientID = "watchdog-agent"
	}

	hostname := toolbox_utils.GetHostname()
	natsCfg.ClientID = fmt.Sprintf("%s-%s", natsCfg.ClientID, hostname)

	prefix := natsCfg.SubjectPrefix
	if prefix == "" {
		prefix = "bastien-antigravity"
	}

	return &BusClient{
		natsCfg:       natsCfg,
		subjectPrefix: prefix,
		hostname:      hostname,
		logger:        l,
	}, nil
}

// SubjectPrefix returns the resolved NATS subject prefix.
func (b *BusClient) SubjectPrefix() string {
	return b.subjectPrefix
}

// Publish sends a payload over NATS using the lock-free active connection pointer.
func (b *BusClient) Publish(subject string, data []byte) error {
	nc := b.conn.Load()
	if nc == nil || nc.Status() != nats.CONNECTED {
		return fmt.Errorf("nats bus not connected")
	}
	return nc.Publish(subject, data)
}

// Start connects to NATS asynchronously with conn_manager backoff and optionally
// publishes periodic node heartbeats until ctx is canceled.
func (b *BusClient) Start(ctx context.Context, publishHeartbeats bool) {
	reconnStrategy := toolbox_conn.NewCriticalStrategy(b.logger)

	var nc *nats.Conn
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return
		}
		conn, err := messaging.Connect(&b.natsCfg, b.logger)
		if err == nil {
			nc = conn
			break
		}
		delay := reconnStrategy.GetNextDelay(attempt)
		supervisor.LogError("watchdog", "Failed to connect to NATS control plane (%v); retrying in %v...", err, delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	b.conn.Store(nc)
	supervisor.LogInfo("watchdog", "Successfully connected NATS Control Plane (prefix: %s)", b.subjectPrefix)
	defer func() {
		b.conn.Store(nil)
		nc.Close()
	}()

	if !publishHeartbeats {
		<-ctx.Done()
		return
	}

	statusSubject := fmt.Sprintf("%s.watchdog.node.%s.status", b.subjectPrefix, b.hostname)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if nc.Status() != nats.CONNECTED {
				continue
			}
			payload := collectServicesStatus(b.hostname)
			if err := nc.Publish(statusSubject, payload); err != nil {
				supervisor.LogError("watchdog", "Failed to publish heartbeat status: %v", err)
			}
		}
	}
}

// -----------------------------------------------------------------------------

func collectServicesStatus(hostname string) []byte {
	type ServiceStatus struct {
		Status string `json:"status"`
		IP     string `json:"ip"`
		Port   string `json:"port"`
		PID    int    `json:"pid,omitempty"`
	}

	type NodeStatus struct {
		Node      string                   `json:"node"`
		Timestamp int64                    `json:"timestamp"`
		Services  map[string]ServiceStatus `json:"services"`
	}

	statusMap := make(map[string]ServiceStatus)
	for _, svc := range supervisor.Services {
		svc.Mu.Lock()
		running := svc.Running
		pid := 0
		if running && svc.Cmd != nil && svc.Cmd.Process != nil {
			pid = svc.Cmd.Process.Pid
		}
		ip := svc.IP
		port := svc.Port
		svc.Mu.Unlock()

		statusStr := "STOPPED"
		if running {
			statusStr = "RUNNING"
		}

		statusMap[svc.Name] = ServiceStatus{
			Status: statusStr,
			IP:     ip,
			Port:   port,
			PID:    pid,
		}
	}

	payloadObj := NodeStatus{
		Node:      hostname,
		Timestamp: time.Now().UTC().Unix(),
		Services:  statusMap,
	}

	data, err := json.Marshal(payloadObj)
	if err != nil {
		return []byte(fmt.Sprintf(`{"node":"%s","error":"json marshal failed"}`, hostname))
	}
	return data
}
