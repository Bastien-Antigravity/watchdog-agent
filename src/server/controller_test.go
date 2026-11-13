package server

import (
	"context"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
)

func TestController_PauseAndResumeService(t *testing.T) {
	// Setup test services in supervisor
	testSvc := &supervisor.Service{
		Name:         "test-svc",
		CapName:      "test_svc",
		MaxCrashes:   3,
		CrashWindow:  30 * time.Second,
		RestartDelay: 3 * time.Second,
	}
	supervisor.Services = []*supervisor.Service{testSvc}

	ctrl := NewController("127.0.0.1:5432", "127.0.0.1:8090")
	ctx := context.Background()

	// Initial status: not paused
	status, err := ctrl.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(status.Services) < 1 {
		t.Fatalf("expected at least 1 service in status")
	}
	if status.Services[0].Paused {
		t.Fatalf("expected service not paused initially")
	}

	// 1. Pause Service
	if err := ctrl.PauseService(ctx, "test-svc"); err != nil {
		t.Fatalf("PauseService failed: %v", err)
	}
	status, _ = ctrl.GetStatus(ctx)
	if !status.Services[0].Paused {
		t.Fatalf("expected service to be paused")
	}

	// Simulate crash loop trip
	testSvc.Mu.Lock()
	testSvc.InCrashLoop = true
	testSvc.CrashTimes = []time.Time{time.Now(), time.Now(), time.Now()}
	testSvc.Mu.Unlock()

	status, _ = ctrl.GetStatus(ctx)
	if !status.Services[0].InCrashLoop || status.Services[0].CrashCount != 3 {
		t.Fatalf("expected InCrashLoop=true and CrashCount=3, got %+v", status.Services[0])
	}

	// 2. Resume Service
	if err := ctrl.ResumeService(ctx, "test-svc"); err != nil {
		t.Fatalf("ResumeService failed: %v", err)
	}
	status, _ = ctrl.GetStatus(ctx)
	if status.Services[0].Paused {
		t.Fatalf("expected service not paused after resume")
	}
	if status.Services[0].InCrashLoop || status.Services[0].CrashCount != 0 {
		t.Fatalf("expected InCrashLoop=false and CrashCount=0 after resume, got %+v", status.Services[0])
	}
}

