package control

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/messaging"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
	"github.com/nats-io/nats.go"
)

// StartNATSControlPlane connects to the NATS event bus and publishes node heartbeats
func StartNATSControlPlane(cfg *config.AppConfig) {
	var natsCfg messaging.NatsConfig
	if err := cfg.Config.GetCapability("nats", &natsCfg); err != nil {
		supervisor.LogError("watchdog", "Failed to resolve NATS capability: %v (running offline)", err)
		return
	}
	if natsCfg.ClientID == "" {
		natsCfg.ClientID = "watchdog-agent"
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost"
	}

	// Append hostname to ClientID to uniquely identify this node
	natsCfg.ClientID = fmt.Sprintf("%s-%s", natsCfg.ClientID, hostname)

	nc, err := messaging.Connect(&natsCfg, nil)
	if err != nil {
		supervisor.LogError("watchdog", "Failed to connect to NATS control plane: %v (running offline)", err)
		return
	}
	defer nc.Close()

	supervisor.LogInfo("watchdog", "Successfully connected NATS Control Plane")

	statusSubject := fmt.Sprintf("antigravity.watchdog.node.%s.status", hostname)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if nc.Status() != nats.CONNECTED {
			continue
		}
		payload := collectServicesStatus(hostname)
		if err := nc.Publish(statusSubject, payload); err != nil {
			supervisor.LogError("watchdog", "Failed to publish heartbeat status: %v", err)
		}
	}
}

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
		Timestamp: time.Now().Unix(),
		Services:  statusMap,
	}

	data, err := json.Marshal(payloadObj)
	if err != nil {
		return []byte(fmt.Sprintf(`{"node":"%s","error":"json marshal failed"}`, hostname))
	}
	return data
}
