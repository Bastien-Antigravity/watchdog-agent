package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
	"github.com/Bastien-Antigravity/watchdog-agent/src/core"
)

type mockWatchdogLogger struct{}

func (m *mockWatchdogLogger) Debug(format string, args ...any)                              {}
func (m *mockWatchdogLogger) Info(format string, args ...any)                               {}
func (m *mockWatchdogLogger) Warning(format string, args ...any)                            {}
func (m *mockWatchdogLogger) Error(format string, args ...any)                              {}
func (m *mockWatchdogLogger) Critical(format string, args ...any)                           {}
func (m *mockWatchdogLogger) Stream(format string, args ...any)                             {}
func (m *mockWatchdogLogger) Logon(format string, args ...any)                              {}
func (m *mockWatchdogLogger) Logout(format string, args ...any)                             {}
func (m *mockWatchdogLogger) Trade(format string, args ...any)                              {}
func (m *mockWatchdogLogger) Schedule(format string, args ...any)                           {}
func (m *mockWatchdogLogger) Report(format string, args ...any)                             {}
func (m *mockWatchdogLogger) GetNotifQueue() <-chan *unilog_interfaces.NotifMessage         { return nil }
func (m *mockWatchdogLogger) SetLocalNotifQueue(ch chan *unilog_interfaces.NotifMessage)    {}
func (m *mockWatchdogLogger) Log(level unilog_interfaces.Level, format string, args ...any) {}
func (m *mockWatchdogLogger) LogWithCaller(level unilog_interfaces.Level, msg, file, line, function, module string) {
}
func (m *mockWatchdogLogger) SetLevel(level unilog_interfaces.Level) {}
func (m *mockWatchdogLogger) GetLevel() unilog_interfaces.Level      { return unilog_interfaces.LevelInfo }
func (m *mockWatchdogLogger) SetCallerSkip(skip int)                 {}
func (m *mockWatchdogLogger) SetMetadata(metadata map[string]string) {}
func (m *mockWatchdogLogger) GetMetadata() map[string]string         { return nil }
func (m *mockWatchdogLogger) AddMetadata(key, value string)          {}
func (m *mockWatchdogLogger) Close()                                 {}

type mockWatchdogControl struct{}

func (m *mockWatchdogControl) GetStatus(ctx context.Context) (core.WatchdogStatusInfo, error) {
	return core.WatchdogStatusInfo{
		Healthy: true,
		Status:  "Running",
		Version: "1.0.0",
		Services: []core.ServiceStatus{
			{Name: "config-server", Running: true, PID: 1001},
		},
	}, nil
}
func (m *mockWatchdogControl) RestartService(ctx context.Context, name string) error { return nil }
func (m *mockWatchdogControl) RestartAll(ctx context.Context) error                  { return nil }
func (m *mockWatchdogControl) PauseService(ctx context.Context, name string) error   { return nil }
func (m *mockWatchdogControl) ResumeService(ctx context.Context, name string) error  { return nil }

func TestWatchdogHandlerContractsEndpoint(t *testing.T) {
	h := NewRESTHandler(&mockWatchdogControl{}, &mockWatchdogLogger{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts", nil)
	rr := httptest.NewRecorder()
	h.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/v1/contracts, got %d", rr.Code)
	}

	var env map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode contracts JSON: %v", err)
	}
	if env["service"] != "watchdog_agent" || env["reliable"] != true || env["exhaustive"] != true {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	contracts, _ := env["contracts"].([]interface{})
	if len(contracts) < 4 {
		t.Fatalf("expected >=4 contract types, got %d", len(contracts))
	}
}

func TestWatchdogHandlerPauseResumeEndpoints(t *testing.T) {
	h := NewRESTHandler(&mockWatchdogControl{}, &mockWatchdogLogger{})

	// Test Pause
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/watchdog/pause", strings.NewReader(`{"name":"config-server"}`))
	pauseRr := httptest.NewRecorder()
	h.Handler().ServeHTTP(pauseRr, pauseReq)
	if pauseRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/v1/watchdog/pause, got %d", pauseRr.Code)
	}
	var pauseResp httpControlResponse
	if err := json.NewDecoder(pauseRr.Body).Decode(&pauseResp); err != nil || !pauseResp.Success {
		t.Fatalf("expected successful pause response, got %+v (err: %v)", pauseResp, err)
	}

	// Test Resume
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/watchdog/resume", strings.NewReader(`{"name":"config-server"}`))
	resumeRr := httptest.NewRecorder()
	h.Handler().ServeHTTP(resumeRr, resumeReq)
	if resumeRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/v1/watchdog/resume, got %d", resumeRr.Code)
	}
	var resumeResp httpControlResponse
	if err := json.NewDecoder(resumeRr.Body).Decode(&resumeResp); err != nil || !resumeResp.Success {
		t.Fatalf("expected successful resume response, got %+v (err: %v)", resumeResp, err)
	}
}
