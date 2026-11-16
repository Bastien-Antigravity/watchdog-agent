package supervisor

// =============================================================================
// ESSENTIAL PROCESS:
// Core domain types, shared supervisor state, and ANSI terminal color definitions
// for watchdog-agent service orchestration.
//
// DATA FLOW:
// 1. Stores singleton Logger instance set during service bootstrap.
// 2. Models Service specifications including build/run commands, dependencies,
//    addresses, and runtime execution handles.
//
// KEY PARAMETERS:
// - Logger: Shared Logger instance for logging across supervisor subsystems.
// - Service: Thread-safe runtime specification for supervised processes.
// =============================================================================

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

var (
	Logger    unilog_ifaces.Logger
	AppConfig *config.AppConfig
)

// -----------------------------------------------------------------------------

// ANSI Color codes for prefixed service logging
const (
	ColorRed          = "\033[1;31m" // Bold Red
	ColorWatchdog     = "\033[1;35m" // Bold Purple
	ColorLogServer    = "\033[1;36m" // Bold Cyan
	ColorConfigServer = "\033[1;32m" // Bold Green
	ColorNotifServer  = "\033[1;33m" // Bold Yellow
	ColorTeleRemote   = "\033[1;34m" // Bold Blue
	ColorRagEngine    = "\033[1;31m" // Bold Red
	ColorRagDashboard = "\033[1;95m" // Bold Light Purple
	ColorWebInterface = "\033[1;94m" // Bold Light Blue
	ColorSquadAgent   = "\033[1;92m" // Bold Light Green
	ColorReset        = "\033[0m"
)

// Service represents a managed service configurations
type Service struct {
	Name         string
	CapName      string
	Path         string
	BuildCmd     string
	BuildArgs    []string
	RunCmd       string
	RunArgs      []string
	Deps         []string
	IP           string
	Port         string
	LogColor     string
	Cmd          *exec.Cmd
	ActiveCmd    atomic.Pointer[exec.Cmd]
	Running      bool
	Paused       bool          // when true, supervisor loop does not auto-restart process
	InCrashLoop  bool          // true if tripped by circuit breaker
	CrashTimes   []time.Time   // history of recent crash timestamps
	MaxCrashes   int           // crash threshold count (default 3)
	CrashWindow  time.Duration // window for crash counting (default 30s)
	RestartDelay time.Duration // delay before restarting (default 3s)
	Mu           sync.Mutex
}

// ResolveAddr dynamically resolves the live IP and Port from AppConfig's atomic LiveConfig,
// falling back to the service's baseline IP and Port.
func (svc *Service) ResolveAddr() (string, string) {
	if AppConfig != nil && svc.CapName != "" && svc.Name != "rag-dashboard" {
		ip, port := resolveServiceAddr(AppConfig, svc.CapName, svc.IP, svc.Port)
		if ip != "" && port != "" {
			return ip, port
		}
	}
	return svc.IP, svc.Port
}
