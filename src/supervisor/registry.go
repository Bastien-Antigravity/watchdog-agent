package supervisor

// =============================================================================
// ESSENTIAL PROCESS:
// Topologies registry and dependency graph validator for managed ecosystem services.
// Configures executable paths, build steps, dynamic capability addresses, and
// inter-service dependency ordering for native host supervisor runs.
//
// DATA FLOW:
// 1. Accepts workspace root directory, host interface defaults, and AppConfig facade.
// 2. Maps service definitions (NATS, TimescaleDB, LogServer, ConfigServer, etc.).
// 3. Dynamically resolves service ports and IP bindings from distributed config.
// 4. Validates dependency DAG for cycles or missing upstream dependencies.
//
// KEY PARAMETERS:
// - rootDir: Absolute filesystem path to the workspace root directory.
// - cfg: AppConfig instance providing dynamic capability addresses.
// =============================================================================

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

// -----------------------------------------------------------------------------

// RegisterServices initializes the topology registry slice of managed services
func RegisterServices(rootDir, defaultHost string, cfg *config.AppConfig) {
	exeExt := ""
	if runtime.GOOS == "windows" {
		exeExt = ".exe"
	}

	// Resolve NATS server executable:
	// 1. Look in system PATH (e.g. brew, apt, winget, choco)
	// 2. Look in workspace docker-deployment/bin/nats-server
	natsRunCmd := ""
	candidates := []string{
		"nats-server" + exeExt,
		"nats-server",
		filepath.Join(rootDir, "docker-deployment", "bin", "nats-server"+exeExt),
		filepath.Join(rootDir, "docker-deployment", "bin", "nats-server"),
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			natsRunCmd = p
			break
		}
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			natsRunCmd = c
			break
		}
	}

	Services = []*Service{
		{
			Name:     "nats-server",
			CapName:  "nats",
			Path:     rootDir,
			BuildCmd: "",
			RunCmd:   natsRunCmd,
			RunArgs:  []string{"-js"},
			Deps:     []string{},
			LogColor: ColorWatchdog,
		},
		{
			Name:     "timescale-db",
			CapName:  "timescale_db",
			Path:     rootDir,
			BuildCmd: "",
			RunCmd:   "",
			RunArgs:  []string{},
			Deps:     []string{},
			LogColor: ColorWatchdog,
		},
		{
			Name:      "log-server",
			CapName:   "log_server",
			Path:      filepath.Join(rootDir, "log-server"),
			BuildCmd:  "cargo",
			BuildArgs: []string{"build"},
			RunCmd:    filepath.Join(rootDir, "log-server", "target", "debug", "log-server"+exeExt),
			RunArgs:   []string{"--name", "log_server"},
			Deps:      []string{},
			LogColor:  ColorLogServer,
		},
		{
			Name:      "config-server",
			CapName:   "config_server",
			Path:      filepath.Join(rootDir, "config-server"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/config-server" + exeExt, "./cmd/config-server"},
			RunCmd:    filepath.Join(rootDir, "config-server", "bin", "config-server"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server"},
			LogColor:  ColorConfigServer,
		},
		{
			Name:      "tele-remote",
			CapName:   "tele_remote",
			Path:      filepath.Join(rootDir, "tele-remote"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/tele-remote" + exeExt, "./cmd/tele-remote"},
			RunCmd:    filepath.Join(rootDir, "tele-remote", "bin", "tele-remote"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server", "config-server"},
			LogColor:  ColorTeleRemote,
		},
		{
			Name:      "web-interface",
			CapName:   "web_interface",
			Path:      filepath.Join(rootDir, "web-interface"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/web-interface" + exeExt, "./cmd/web-interface"},
			RunCmd:    filepath.Join(rootDir, "web-interface", "bin", "web-interface"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{}, // Starts independently so dashboard is immediately visible even if microservices haven't started
			LogColor:  ColorWebInterface,
		},
		{
			Name:      "notif-server",
			CapName:   "notif_server",
			Path:      filepath.Join(rootDir, "notif-server"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/notif-server" + exeExt, "./cmd/notif-server"},
			RunCmd:    filepath.Join(rootDir, "notif-server", "bin", "notif-server"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server", "config-server", "tele-remote"},
			LogColor:  ColorNotifServer,
		},
		{
			Name:     "rag-engine",
			CapName:  "rag_engine",
			Path:     filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine")),
			RunArgs:  []string{"main.py", "server"},
			Deps:     []string{"log-server", "config-server", "timescale-db"},
			LogColor: ColorRagEngine,
		},
		{
			Name:     "rag-dashboard",
			CapName:  "rag_engine",
			Path:     filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine")),
			RunArgs:  []string{"main.py", "dashboard"},
			Deps:     []string{"log-server", "config-server", "rag-engine"},
			LogColor: ColorRagDashboard,
		},
		{
			Name:     "squad-agent",
			CapName:  "squad_agent",
			Path:     filepath.Join(rootDir, "obsidian-brain", "08-Squad-Agent"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "08-Squad-Agent")),
			RunArgs:  []string{"main.py", "squad-agent"},
			Deps:     []string{"log-server", "config-server", "nats-server"},
			LogColor: ColorSquadAgent,
		},
	}

	// Filter and initialize addresses for services
	for _, svc := range Services {
		var defaultPort string
		switch svc.Name {
		case "timescale-db":
			defaultPort = "5432"
		case "log-server":
			defaultPort = "9020"
		case "config-server":
			defaultPort = "3306"
		case "notif-server":
			defaultPort = "1026"
		case "tele-remote":
			defaultPort = "1863"
		case "web-interface":
			defaultPort = "5000"
		case "rag-engine":
			defaultPort = "8090"
		case "rag-dashboard":
			defaultPort = "8082"
		case "nats-server":
			defaultPort = "4222"
		case "squad-agent":
			defaultPort = "8085"
		}

		svc.IP, svc.Port = resolveServiceAddr(cfg, svc.CapName, defaultHost, defaultPort)
		if svc.Name == "nats-server" {
			httpPort := "8222"
			serverName := "nats-server"
			storeDir := filepath.Join(rootDir, "docker-deployment", "data", "jetstream")

			var tlsEnabled bool
			var tlsCert, tlsKey, tlsCA string
			var tlsVerify bool

			if capMap, ok := cfg.Capabilities["nats"].(map[string]interface{}); ok {
				if hp, exists := capMap["http_port"]; exists && hp != nil && fmt.Sprintf("%v", hp) != "" {
					httpPort = fmt.Sprintf("%v", hp)
				}
				if sn, exists := capMap["server_name"]; exists && sn != nil && fmt.Sprintf("%v", sn) != "" {
					serverName = fmt.Sprintf("%v", sn)
				}
				if js, ok := capMap["jetstream"].(map[string]interface{}); ok {
					if sd, exists := js["store_dir"]; exists && sd != nil && fmt.Sprintf("%v", sd) != "" {
						sdStr := fmt.Sprintf("%v", sd)
						if filepath.IsAbs(sdStr) {
							storeDir = sdStr
						} else {
							storeDir = filepath.Join(rootDir, sdStr)
						}
					}
				}
				if tlsMap, ok := capMap["tls"].(map[string]interface{}); ok {
					if en, exists := tlsMap["enabled"]; exists && (en == true || fmt.Sprintf("%v", en) == "true") {
						tlsEnabled = true
					}
					if cert, exists := tlsMap["cert_file"]; exists && cert != nil {
						tlsCert = fmt.Sprintf("%v", cert)
					}
					if key, exists := tlsMap["key_file"]; exists && key != nil {
						tlsKey = fmt.Sprintf("%v", key)
					}
					if ca, exists := tlsMap["ca_file"]; exists && ca != nil {
						tlsCA = fmt.Sprintf("%v", ca)
					}
					if ver, exists := tlsMap["verify"]; exists && (ver == true || fmt.Sprintf("%v", ver) == "true") {
						tlsVerify = true
					}
				}
			}

			// If binding outside loopback (non-127.0.0.1 / localhost) and certs are defined, or explicitly enabled
			isOutsideLoopback := svc.IP != "127.0.0.1" && svc.IP != "localhost" && svc.IP != ""
			if (isOutsideLoopback && tlsCert != "" && tlsKey != "") || tlsEnabled {
				tlsEnabled = true
			}

			args := []string{"-js", "-sd", storeDir, "-p", svc.Port, "-a", svc.IP, "-m", httpPort, "-n", serverName}
			if tlsEnabled {
				args = append(args, "--tls")
				if tlsCert != "" {
					if !filepath.IsAbs(tlsCert) {
						tlsCert = filepath.Join(rootDir, tlsCert)
					}
					args = append(args, "--tlscert", tlsCert)
				}
				if tlsKey != "" {
					if !filepath.IsAbs(tlsKey) {
						tlsKey = filepath.Join(rootDir, tlsKey)
					}
					args = append(args, "--tlskey", tlsKey)
				}
				if tlsVerify {
					args = append(args, "--tlsverify")
				}
				if tlsCA != "" {
					if !filepath.IsAbs(tlsCA) {
						tlsCA = filepath.Join(rootDir, tlsCA)
					}
					args = append(args, "--tlscacert", tlsCA)
				}
			}
			svc.RunArgs = args
		}
		if svc.Name == "rag-dashboard" {
			if capMap, ok := cfg.Capabilities["rag_engine"].(map[string]interface{}); ok {
				if dash, ok := capMap["dashboard"].(map[string]interface{}); ok {
					if port, exists := dash["port"]; exists {
						svc.Port = fmt.Sprintf("%v", port)
					}
				}
			}
		}
	}
}

func resolveServiceAddr(cfg *config.AppConfig, capName string, defaultIP, defaultPort string) (string, string) {
	// Special Case for RAG Engine MCP address resolution
	if capName == "rag_engine" && defaultPort == "8090" {
		if capMap, ok := cfg.Capabilities["rag_engine"].(map[string]interface{}); ok {
			if mcp, ok := capMap["mcp"].(map[string]interface{}); ok {
				var resIP, resPort string
				if ip, exists := mcp["ip"]; exists {
					resIP = fmt.Sprintf("%v", ip)
				}
				if port, exists := mcp["port"]; exists {
					resPort = fmt.Sprintf("%v", port)
				}
				if resIP != "" && resPort != "" {
					return resIP, resPort
				}
			}
		}
	}

	addr, err := cfg.GetListenAddr(capName)
	if err == nil && addr != "" {
		if host, port, splitErr := net.SplitHostPort(addr); splitErr == nil {
			return host, port
		}
	}
	return defaultIP, defaultPort
}

// ValidateRegistry checks for missing dependencies and dependency cycles
func ValidateRegistry() error {
	// 1. Check for missing dependencies
	for _, svc := range Services {
		for _, depName := range svc.Deps {
			if FindServiceByName(depName) == nil {
				return fmt.Errorf("service '%s' depends on unknown service '%s'", svc.Name, depName)
			}
		}
	}

	// 2. Check for dependency cycles using DFS (color-based)
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var dfs func(name string) error
	dfs = func(name string) error {
		visited[name] = 1 // visiting
		svc := FindServiceByName(name)
		if svc != nil {
			for _, depName := range svc.Deps {
				state := visited[depName]
				if state == 1 {
					return fmt.Errorf("dependency cycle detected involving service '%s'", depName)
				} else if state == 0 {
					if err := dfs(depName); err != nil {
						return err
					}
				}
			}
		}
		visited[name] = 2 // fully visited
		return nil
	}

	for _, svc := range Services {
		if visited[svc.Name] == 0 {
			if err := dfs(svc.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
