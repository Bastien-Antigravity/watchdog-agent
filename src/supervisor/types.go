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
// - Logger: Shared ILogger instance for logging across supervisor subsystems.
// - Service: Thread-safe runtime specification for supervised processes.
// =============================================================================

import (
	"os/exec"
	"sync"

	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

var Logger unilog_ifaces.Logger

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
	ColorStartSquad   = "\033[1;92m" // Bold Light Green
	ColorReset        = "\033[0m"
)

// Service represents a managed service configurations
type Service struct {
	Name      string
	CapName   string
	Path      string
	BuildCmd  string
	BuildArgs []string
	RunCmd    string
	RunArgs   []string
	Deps      []string
	IP        string
	Port      string
	LogColor  string
	Cmd       *exec.Cmd
	Running   bool
	Mu        sync.Mutex
}
