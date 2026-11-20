package main

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for watchdog-agent MFE URL normalization in Docker environments.
//
// DATA FLOW:
// 1. Input: Docker-internal REST address (log-shipper:9095 / 0.0.0.0:9095) and AppConfig.
// 2. Logic: Invokes buildMFEURL with DOCKER_ENV=true and common.public_ip / HOST_IP.
// 3. Output: Asserts browser-reachable MFE URL resolution without silent fallbacks.
//
// KEY PARAMETERS:
// - t: Standard Go testing context.
// =============================================================================

import (
	"os"
	"path/filepath"
	"testing"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
)

// -----------------------------------------------------------------------------

func TestBuildMFEURL_DockerResolution(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "watchdog-docker.yaml")
	yamlContent := `common:
  name: watchdog-agent
  public_ip: 127.0.0.2
capabilities:
  watchdog_agent:
    ip: log-shipper
    port: 9090
    rest_port: 9095
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	t.Setenv("CONFIG_PATH", yamlPath)
	t.Setenv("SHARED_CONFIG_PATH", "")
	t.Setenv("HOST_IP", "")
	t.Setenv("DOCKER_ENV", "true")

	cfg, err := toolbox_config.LoadConfig("standalone", nil)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if got := buildMFEURL(cfg, "log-shipper:9095"); got != "http://127.0.0.2:9095/static/mfe.js" {
		t.Fatalf("expected http://127.0.0.2:9095/static/mfe.js, got %q", got)
	}

	if got := buildMFEURL(cfg, "0.0.0.0:9095"); got != "http://127.0.0.2:9095/static/mfe.js" {
		t.Fatalf("expected http://127.0.0.2:9095/static/mfe.js for wildcard bind, got %q", got)
	}

	t.Setenv("HOST_IP", "203.0.113.20")
	if got := buildMFEURL(cfg, "log-shipper:9095"); got != "http://203.0.113.20:9095/static/mfe.js" {
		t.Fatalf("expected HOST_IP override http://203.0.113.20:9095/static/mfe.js, got %q", got)
	}
}
