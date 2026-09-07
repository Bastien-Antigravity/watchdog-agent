package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	toolbox_bootstrap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/bootstrap"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	watchdog_config "github.com/Bastien-Antigravity/watchdog-agent/src/config"
	"github.com/Bastien-Antigravity/watchdog-agent/src/control"
	"github.com/Bastien-Antigravity/watchdog-agent/src/rest"
	"github.com/Bastien-Antigravity/watchdog-agent/src/server"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
	"github.com/Bastien-Antigravity/watchdog-agent/src/telegram"
	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

func main() {
	profileFlag := flag.String("profile", "standalone", "Configuration profile to load (e.g. standalone, production)")
	buildFlag := flag.Bool("build", true, "Automatically build binaries on startup")
	flag.Parse()

	supervisor.LogInfo("watchdog", "Starting Bastien-Antigravity Watchdog Agent (Profile: %s)...", *profileFlag)

	// Resolve workspace root dynamically
	rootDir, err := utils.FindWorkspaceRoot()
	if err != nil {
		supervisor.LogError("watchdog", "Failed to find workspace root: %v", err)
		os.Exit(1)
	}
	supervisor.LogInfo("watchdog", "Resolved workspace root: %s", rootDir)

	// Heal config symlinks BEFORE loading configuration
	if err := watchdog_config.HealSymlinks(rootDir); err != nil {
		supervisor.LogError("watchdog", "Failed to heal ecosystem config symlinks: %v", err)
		os.Exit(1)
	}

	// Load ecosystem configuration & Initialize Logger
	cfg, appLogger := toolbox_bootstrap.BootstrapService("watchdog-agent")
	defer appLogger.Close()
	supervisor.Logger = appLogger

	// Detect local host IPs to determine local service assignments
	localIPs, err := utils.GetLocalIPs()
	if err != nil {
		appLogger.Critical("Failed to detect local IPs: %v", err)
		os.Exit(1)
	}

	// Single instance locking
	lockPath := filepath.Join(rootDir, ".watchdog-agent.lock")
	lockFile, err := utils.AcquireLock(lockPath)
	if err != nil {
		appLogger.Critical("Another instance of watchdog-agent is already running (failed to acquire lock). Exiting.")
		os.Exit(1)
	}
	defer lockFile.Close()

	// Resolve Vector DB Port from capabilities
	vectorDBPort := "8000"
	if capMap, ok := cfg.Capabilities["rag_engine"].(map[string]interface{}); ok {
		if vdb, ok := capMap["vector_db"].(map[string]interface{}); ok {
			if port, exists := vdb["port"]; exists {
				vectorDBPort = fmt.Sprintf("%v", port)
			}
		}
	}

	defaultHost := os.Getenv("HOST_IP")
	if defaultHost == "" {
		defaultHost = "127.0.0.1"
	}

	// Initialize the topologies registry of services
	supervisor.RegisterServices(rootDir, defaultHost, vectorDBPort, cfg)

	// Validate registry topologies
	if err := supervisor.ValidateRegistry(); err != nil {
		appLogger.Critical("Ecosystem registry validation failed: %v", err)
		os.Exit(1)
	}

	// Base environment to inject to children (filter out watchdog-specific overrides)
	var filteredEnv []string
	for _, envVar := range os.Environ() {
		if !strings.HasPrefix(envVar, "LOGGER_PROFILE=") {
			filteredEnv = append(filteredEnv, envVar)
		}
	}
	baseEnv := filteredEnv

	// Inherit or inject default MFE and Base Scripts configurations
	wbIP := os.Getenv("WB_IP")
	if wbIP == "" {
		wbIP = defaultHost
	}
	wbPort := os.Getenv("WB_PORT")
	if wbPort == "" {
		wbPort = "5000"
	}
	wbGrpcIP := os.Getenv("WB_GRPC_IP")
	if wbGrpcIP == "" {
		wbGrpcIP = defaultHost
	}
	wbGrpcPort := os.Getenv("WB_GRPC_PORT")
	if wbGrpcPort == "" {
		wbGrpcPort = "8001"
	}
	bsIP := os.Getenv("BS_IP")
	if bsIP == "" {
		bsIP = defaultHost
	}
	bsPort := os.Getenv("BS_PORT")
	if bsPort == "" {
		bsPort = "8085"
	}

	baseEnv = append(baseEnv,
		fmt.Sprintf("WB_IP=%s", wbIP),
		fmt.Sprintf("WB_PORT=%s", wbPort),
		fmt.Sprintf("WB_GRPC_IP=%s", wbGrpcIP),
		fmt.Sprintf("WB_GRPC_PORT=%s", wbGrpcPort),
		fmt.Sprintf("BS_IP=%s", bsIP),
		fmt.Sprintf("BS_PORT=%s", bsPort),
	)

	// Compile local binaries if required (e.g. if --build=true)
	if *buildFlag {
		for _, svc := range supervisor.Services {
			if svc.BuildCmd != "" {
				appLogger.Info("Building service '%s'...", svc.Name)
				cmd := exec.Command(svc.BuildCmd, svc.BuildArgs...)
				cmd.Dir = svc.Path
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					appLogger.Error("Build failed for '%s': %v", svc.Name, err)
					os.Exit(1)
				}
				appLogger.Info("Service '%s' compiled successfully.", svc.Name)
			}
		}
	}

	// Launch each service in its own monitor goroutine
	for _, svc := range supervisor.Services {
		// Only run if the service IP is resolved to this local node
		if utils.IsLocal(svc.IP, localIPs) {
			// Clone baseEnv slice to prevent concurrent slice modification races
			svcEnv := make([]string, len(baseEnv))
			copy(svcEnv, baseEnv)
			go supervisor.MonitorAndSupervise(svc, svcEnv, localIPs)
		} else {
			appLogger.Info("Skipping non-local service '%s' assigned to node %s", svc.Name, svc.IP)
		}
	}

	// Connect NATS Control Plane heartbeat event loop
	go control.StartNATSControlPlane(cfg)

	// Resolve timescale_db address using capability configuration
	postgresAddr, err := cfg.GetListenAddr("timescale_db")
	if err != nil || postgresAddr == "" {
		appLogger.Error("Postgres database address capability not configured: %v", err)
	} else {
		go supervisor.LaunchPostgresAttempt(postgresAddr)
	}

	// Resolve RAG MCP address using capability configuration
	ragMcpAddr := ""
	var ragCap struct {
		MCP struct {
			IP   string `json:"ip"`
			Port string `json:"port"`
		} `json:"mcp"`
	}
	if err := cfg.GetCapability("rag_engine", &ragCap); err == nil {
		if ragCap.MCP.IP != "" && ragCap.MCP.Port != "" {
			ragMcpAddr = net.JoinHostPort(ragCap.MCP.IP, ragCap.MCP.Port)
		}
	}
	if ragMcpAddr == "" {
		appLogger.Error("RAG MCP capability address not configured")
	}

	// Initialize Watchdog Controller
	ctrl := server.NewController(postgresAddr, ragMcpAddr)

	// Resolve REST address and Start REST Server
	restAddr, err := cfg.GetRESTAddr("watchdog_agent")
	if err != nil {
		appLogger.Critical("Failed to resolve REST address for watchdog_agent: %v", err)
		os.Exit(1)
	}
	parts := strings.SplitN(restAddr, ":", 2)
	var restPort int
	if len(parts) != 2 {
		appLogger.Critical("Invalid REST address format: '%s'", restAddr)
		os.Exit(1)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &restPort); err != nil {
		appLogger.Critical("Failed to parse REST port from '%s': %v", restAddr, err)
		os.Exit(1)
	}

	restHandler := rest.NewRESTHandler(ctrl, appLogger)
	go func() {
		if err := restHandler.StartServer(restPort); err != nil {
			appLogger.Error("REST server failed to start: %v", err)
		}
	}()

	// Register watchdog OpenMFE with web-interface dynamically
	go func() {
		mfeUrl := fmt.Sprintf("http://%s/static/mfe.js", restAddr)
		payload := fmt.Sprintf(`{
			"name": "watchdog-agent",
			"tag": "watchdog-agent-mfe",
			"url": "%s",
			"navTitle": "🤖 Watchdog"
		}`, mfeUrl)

		client := &http.Client{Timeout: 3 * time.Second}
		for i := 0; i < 15; i++ {
			var webIP, webPort string
			webSvc := supervisor.FindServiceByName("web-interface")
			if webSvc != nil {
				webIP = webSvc.IP
				webPort = webSvc.Port
			} else {
				webIP = "127.0.0.1"
				webPort = "5000"
			}
			regUrl := fmt.Sprintf("http://%s:%s/api/v1/register", webIP, webPort)

			resp, err := client.Post(regUrl, "application/json", strings.NewReader(payload))
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					appLogger.Info("Successfully registered watchdog OpenMFE with web-interface at %s", regUrl)
					return
				}
				appLogger.Warning("OpenMFE registration attempt %d failed with status %s", i+1, resp.Status)
			} else {
				appLogger.Warning("OpenMFE registration attempt %d failed: %v", i+1, err)
			}
			time.Sleep(3 * time.Second)
		}
		appLogger.Warning("Failed to register watchdog OpenMFE with web-interface after 15 attempts")
	}()

	// Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(appLogger)

	// Setup Telegram client
	telegram.SetupTelegram(cfg, ctrl, appLogger, lm)

	// Register cleanup hooks
	lm.Register("StopServices", func() error {
		appLogger.Info("Shutdown signal received. Terminating all managed processes...")
		supervisor.KillAll()
		return nil
	})

	lm.Wait(context.Background())
}
