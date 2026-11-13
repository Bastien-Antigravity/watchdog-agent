package shipper

import (
	"testing"
)

func TestParseLogLine_FixedFormat(t *testing.T) {
	line := "2026-11-08T15:20:00.123456000Z    iMac.local   web-interface          ERROR      middleware.go        LoggerMiddleware          24     Database connection timed out [metadata: pid=82164 pname=web-interface]"

	log, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("expected log line to parse successfully")
	}

	if log.Level != 10 || log.LevelStr != "ERROR" {
		t.Errorf("expected level 10 ERROR, got %d %s", log.Level, log.LevelStr)
	}
	if log.ServiceName != "web-interface" {
		t.Errorf("expected service web-interface, got %q", log.ServiceName)
	}
	if log.Hostname != "iMac.local" {
		t.Errorf("expected hostname iMac.local, got %q", log.Hostname)
	}
	if log.Filename != "middleware.go" {
		t.Errorf("expected filename middleware.go, got %q", log.Filename)
	}
	if log.FunctionName != "LoggerMiddleware" {
		t.Errorf("expected func LoggerMiddleware, got %q", log.FunctionName)
	}
	if log.LineNumber != "24" {
		t.Errorf("expected line 24, got %q", log.LineNumber)
	}
	if log.Message != "Database connection timed out [metadata: pid=82164 pname=web-interface]" {
		t.Errorf("unexpected message: %q", log.Message)
	}
	if log.Timestamp.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", log.Timestamp.Year())
	}
}

func TestParseLogLine_CriticalSeverity(t *testing.T) {
	line := "2026-11-08T15:25:00.000000000Z    node-01      squad-agent            CRITICAL   brain.py             init_vault                105    Catastrophic KMS failure"

	log, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("expected critical line to parse")
	}
	if log.Level != 11 || log.LevelStr != "CRITICAL" {
		t.Errorf("expected level 11 CRITICAL, got %d %s", log.Level, log.LevelStr)
	}
}

func TestParseLogLine_IgnoreInfoAndWarningAndMentionedError(t *testing.T) {
	infoLine := "2026-11-08T15:20:00.000000000Z    iMac.local   web-interface          INFO       main.go              main                      50     Starting service"
	warnLine := "2026-11-08T15:20:00.000000000Z    iMac.local   web-interface          WARNING    main.go              main                      55     Slow connection"
	infoMention := "2026-11-08T15:20:00.000000000Z    iMac.local   web-interface          INFO       retry.go             Retry                     60     Recovered after previous ERROR cleanly"

	if _, ok := ParseLogLine(infoLine); ok {
		t.Errorf("INFO line should not be accepted")
	}
	if _, ok := ParseLogLine(warnLine); ok {
		t.Errorf("WARNING line should not be accepted")
	}
	if _, ok := ParseLogLine(infoMention); ok {
		t.Errorf("INFO line merely mentioning ERROR in message body should not be accepted")
	}
}

func TestParseLogLine_FallbackRegex(t *testing.T) {
	line := "2026-11-08T15:20:00Z host-a my-custom-service ERROR err.go HandleError 99 Failed to send packet"

	log, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("expected fallback regex to parse line")
	}
	if log.ServiceName != "my-custom-service" {
		t.Errorf("expected my-custom-service, got %q", log.ServiceName)
	}
	if log.Level != 10 {
		t.Errorf("expected level 10, got %d", log.Level)
	}
}

func TestParseLogLine_ShortLineWithKeyword(t *testing.T) {
	shortLine := "2026-11-09T14:00:00Z node-1 test-svc ERROR main.go Run 1 Short error"
	log, ok := ParseLogLine(shortLine)
	if !ok {
		t.Fatalf("expected short error line to parse successfully")
	}
	if log.ServiceName != "test-svc" || log.Message != "Short error" {
		t.Errorf("unexpected parsed values: svc=%q msg=%q", log.ServiceName, log.Message)
	}
}

func TestParseLogLine_StackTraceMetadata(t *testing.T) {
	line := "2026-11-08T15:20:00.000000000Z    node-01      orderbook              ERROR      engine.go            ProcessOrder              88     Order processing failed [metadata: pid=123 stack=Traceback: nil pointer dereference]"
	log, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("expected line to parse")
	}
	if log.StackTrace == "" {
		t.Error("expected stack trace to be extracted from metadata tag")
	}
}

func TestParseLogLine_RejectsInvalidTimestamp(t *testing.T) {
	line := "INVALID_TIMESTAMP_STRING           node-01      service-x              ERROR      main.go              Start                     12     Syntax error"
	if _, ok := ParseLogLine(line); ok {
		t.Fatalf("expected malformed timestamp line to be rejected instead of synthesizing time.Now()")
	}
}

func TestParseLogLine_SanitizesPoisonPillUTF8AndNUL(t *testing.T) {
	line := "2026-11-08T15:20:00.000000000Z    node-01      service-x              ERROR      main.go              Start                     12     Corrupt payload \x00 \xff\xfe end"
	log, ok := ParseLogLine(line)
	if !ok {
		t.Fatalf("expected sanitized line to parse")
	}
	for _, r := range log.Message {
		if r == '\x00' {
			t.Fatal("expected NUL byte to be stripped from parsed message")
		}
	}
}

func TestParseLogLine_CloudJSONFormat(t *testing.T) {
	errJSON := `{"timestamp":"2026-11-11T19:15:00.123456Z","level":10,"message":"Failed to connect to config-server","hostname":"rag-engine-docker","loggerName":"rag-engine","filename":"main.py","functionName":"startup","lineNumber":"42","stackTrace":"Traceback: connection refused"}`
	log, ok := ParseLogLine(errJSON)
	if !ok {
		t.Fatalf("expected cloud JSON ERROR line (level 10) to parse")
	}
	if log.Level != 10 || log.LevelStr != "ERROR" || log.ServiceName != "rag-engine" || log.Hostname != "rag-engine-docker" {
		t.Errorf("unexpected parsed JSON fields: %+v", log)
	}
	if log.StackTrace != "Traceback: connection refused" {
		t.Errorf("expected stackTrace preserved, got %q", log.StackTrace)
	}

	critJSON := `{"timestamp":"2026-11-11T19:15:01Z","level":11,"message":"Vault key unreadable","hostname":"squad-agent-docker","loggerName":"squad-agent","serviceName":"squad_agent","filename":"crypto.py","functionName":"unlock","lineNumber":"18"}`
	crit, ok := ParseLogLine(critJSON)
	if !ok || crit.Level != 11 || crit.LevelStr != "CRITICAL" || crit.ServiceName != "squad_agent" {
		t.Fatalf("expected cloud JSON CRITICAL line (level 11) with serviceName override, got ok=%v %+v", ok, crit)
	}

	infoJSON := `{"timestamp":"2026-11-11T19:15:02Z","level":3,"message":"Service ready ERROR word in message","hostname":"web-interface","loggerName":"web-interface"}`
	if _, ok := ParseLogLine(infoJSON); ok {
		t.Errorf("expected cloud JSON INFO line (level 3) to be ignored")
	}

	badTSJSON := `{"timestamp":"not-a-timestamp","level":10,"message":"Broken timestamp","hostname":"svc","loggerName":"svc"}`
	if _, ok := ParseLogLine(badTSJSON); ok {
		t.Errorf("expected cloud JSON with invalid timestamp to be rejected")
	}
}
