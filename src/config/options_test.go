package config

import (
	"os"
	"testing"
)

func TestResolveRuntimeOptions_NativeDefault(t *testing.T) {
	_ = os.Unsetenv("DOCKER_ENV")
	_ = os.Unsetenv("WATCHDOG_MODE")
	_ = os.Unsetenv("WATCHDOG_SUPERVISE")
	_ = os.Unsetenv("WATCHDOG_SHIPPER")
	_ = os.Unsetenv("WATCHDOG_REST")

	opts := ResolveRuntimeOptions("", true, true, true, true, "standalone")
	if opts.Mode != "all" {
		t.Errorf("expected mode 'all', got %s", opts.Mode)
	}
	if !opts.Supervise {
		t.Error("expected Supervise to be true in native default mode")
	}
	if !opts.Shipper {
		t.Error("expected Shipper to be true in native default mode")
	}
	if !opts.REST {
		t.Error("expected REST to be true in native default mode")
	}
}

func TestResolveRuntimeOptions_DockerAutoDetect(t *testing.T) {
	_ = os.Setenv("DOCKER_ENV", "true")
	defer os.Unsetenv("DOCKER_ENV")
	_ = os.Unsetenv("WATCHDOG_MODE")
	_ = os.Unsetenv("WATCHDOG_SUPERVISE")

	opts := ResolveRuntimeOptions("", true, true, true, true, "standalone")
	if opts.Mode != "shipper" {
		t.Errorf("expected mode 'shipper' in Docker default, got %s", opts.Mode)
	}
	if opts.Supervise {
		t.Error("expected Supervise to be false inside Docker")
	}
	if !opts.Shipper {
		t.Error("expected Shipper to be true inside Docker")
	}
	if opts.REST {
		t.Error("expected REST to be false inside Docker shipper mode")
	}
}

func TestResolveRuntimeOptions_ExplicitShipperMode(t *testing.T) {
	_ = os.Unsetenv("DOCKER_ENV")

	opts := ResolveRuntimeOptions("shipper", true, true, true, false, "production")
	if opts.Mode != "shipper" {
		t.Errorf("expected mode 'shipper', got %s", opts.Mode)
	}
	if opts.Supervise {
		t.Error("expected Supervise to be false in explicit shipper mode")
	}
	if !opts.Shipper {
		t.Error("expected Shipper to be true in explicit shipper mode")
	}
	if opts.REST {
		t.Error("expected REST to be false in explicit shipper mode")
	}
}

func TestResolveRuntimeOptions_ExplicitSupervisorMode(t *testing.T) {
	opts := ResolveRuntimeOptions("supervisor", true, true, true, false, "production")
	if opts.Mode != "supervisor" {
		t.Errorf("expected mode 'supervisor', got %s", opts.Mode)
	}
	if !opts.Supervise {
		t.Error("expected Supervise to be true in supervisor mode")
	}
	if opts.Shipper {
		t.Error("expected Shipper to be false in supervisor mode")
	}
}

func TestResolveRuntimeOptions_EnvOverrides(t *testing.T) {
	_ = os.Setenv("WATCHDOG_SUPERVISE", "false")
	defer os.Unsetenv("WATCHDOG_SUPERVISE")
	_ = os.Setenv("WATCHDOG_SHIPPER", "true")
	defer os.Unsetenv("WATCHDOG_SHIPPER")

	opts := ResolveRuntimeOptions("all", true, true, true, false, "standalone")
	if opts.Supervise {
		t.Error("expected Supervise to be false when overridden by WATCHDOG_SUPERVISE=false")
	}
	if !opts.Shipper {
		t.Error("expected Shipper to be true")
	}
}
