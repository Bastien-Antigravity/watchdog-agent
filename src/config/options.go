package config

// =============================================================================
// ESSENTIAL PROCESS:
// Resolves and normalizes runtime operational modes and feature flags for the
// watchdog-agent across native OS, Docker containerized, and distributed stacks.
//
// DATA FLOW:
// 1. Accepts CLI flags for mode, profile, supervise, shipper, rest, and build.
// 2. Evaluates environment variables (WATCHDOG_MODE, DOCKER_ENV, etc.).
// 3. Applies intelligent defaults (e.g. disables child supervision in Docker).
// 4. Returns an immutable RuntimeOptions struct for agent bootstrap.
//
// KEY PARAMETERS:
// - Mode: Operational role ('all', 'supervisor', 'shipper', 'telemetry').
// - Supervise: Flag enabling child process management and crash restart loops.
// - Shipper: Flag enabling autonomous error log tailing to TimescaleDB.
// - REST: Flag enabling HTTP management API and embedded OpenMFE dashboard.
// =============================================================================

import (
	"os"
	"strings"
)

// -----------------------------------------------------------------------------

// RuntimeOptions encapsulates all resolved feature switches and runtime modes.
type RuntimeOptions struct {
	Mode      string
	Supervise bool
	Shipper   bool
	REST      bool
	Build     bool
	Profile   string
	IsDocker  bool
}

// -----------------------------------------------------------------------------

// ResolveRuntimeOptions evaluates CLI parameters and environment variables to construct
// the authoritative execution capabilities of the watchdog-agent.
func ResolveRuntimeOptions(
	modeFlag string,
	superviseFlag bool,
	shipperFlag bool,
	restFlag bool,
	buildFlag bool,
	profileFlag string,
) RuntimeOptions {
	isDocker := os.Getenv("DOCKER_ENV") == "true"

	mode := strings.ToLower(modeFlag)
	if mode == "" {
		mode = strings.ToLower(os.Getenv("WATCHDOG_MODE"))
	}
	if mode == "" {
		if isDocker {
			mode = "shipper"
		} else {
			mode = "all"
		}
	}

	supervise := superviseFlag
	shipper := shipperFlag
	rest := restFlag

	switch mode {
	case "shipper":
		supervise = false
		shipper = true
		rest = false
	case "supervisor":
		supervise = true
		shipper = false
	case "telemetry":
		supervise = false
		shipper = false
		rest = true
	case "all":
		if isDocker {
			supervise = false // In Docker, Docker daemon is the authoritative process restarter
		}
	}

	// Explicit environment overrides take ultimate precedence
	if val := os.Getenv("WATCHDOG_SUPERVISE"); val != "" {
		supervise = val == "true" || val == "1"
	}
	if val := os.Getenv("WATCHDOG_SHIPPER"); val != "" {
		shipper = val == "true" || val == "1"
	}
	if val := os.Getenv("WATCHDOG_REST"); val != "" {
		rest = val == "true" || val == "1"
	}

	return RuntimeOptions{
		Mode:      mode,
		Supervise: supervise,
		Shipper:   shipper,
		REST:      rest,
		Build:     buildFlag,
		Profile:   profileFlag,
		IsDocker:  isDocker,
	}
}

