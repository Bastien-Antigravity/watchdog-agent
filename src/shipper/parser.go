package shipper

// =============================================================================
// ESSENTIAL PROCESS:
// High-performance parser for fleet standardized 8-column log text lines.
// Identifies and extracts operational failures (level >= ERROR) into structured
// telemetry models for TimescaleDB ingestion.
//
// DATA FLOW:
// 1. Sanitizes input text (valid UTF-8, strips NUL bytes) and applies fast level check.
// 2. Extracts fixed-width columns (timestamp, host, logger, level, file, func, line, msg)
//    with regex fallback for variably-spaced lines.
// 3. Strictly parses RFC3339/RFC3339Nano UTC timestamps without synthesizing fake times.
// 4. Returns ParsedErrorLog or false if non-error / malformed.
//
// KEY PARAMETERS:
// - line: Raw newline-delimited log entry from a local microservice log file.
// =============================================================================

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------

// ParsedErrorLog represents a structured error event ready for TimescaleDB storage.
type ParsedErrorLog struct {
	Timestamp    time.Time `json:"timestamp"`
	BrainID      string    `json:"brain_id"`
	Hostname     string    `json:"hostname"`
	ServiceName  string    `json:"service_name"`
	LoggerName   string    `json:"logger_name"`
	Level        int       `json:"level"`     // 10 = ERROR, 11 = CRITICAL
	LevelStr     string    `json:"level_str"` // "ERROR", "CRITICAL"
	Filename     string    `json:"filename"`
	FunctionName string    `json:"function_name"`
	LineNumber   string    `json:"line_number"`
	Message      string    `json:"message"`
	StackTrace   string    `json:"stack_trace,omitempty"`
	EventHash    string    `json:"event_hash"`
}

var fallbackLogRegex = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(ERROR|CRITICAL)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$`)

// -----------------------------------------------------------------------------

func sanitizeLogText(s string) string {
	cleaned := strings.ToValidUTF8(s, "?")
	if strings.ContainsRune(cleaned, '\x00') {
		cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	}
	return cleaned
}

func parseCanonicalLevel(levelStr string) (int, bool) {
	switch levelStr {
	case "ERROR":
		return 10, true
	case "CRITICAL":
		return 11, true
	default:
		return 0, false
	}
}

func parseLogTimestamp(tsStr string) (time.Time, bool) {
	if ts, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
		return ts.UTC(), true
	}
	if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
		return ts.UTC(), true
	}
	return time.Time{}, false
}

func extractStackTrace(msgPart string) string {
	if idx := strings.Index(msgPart, "[metadata:"); idx != -1 {
		meta := msgPart[idx:]
		if strings.Contains(meta, "stack=") {
			return meta
		}
	}
	return ""
}

// ComputeEventHash derives a deterministic SHA-256 deduplication key scoped to
// brain, file path, byte offset, and timestamp so crash replays are idempotent.
func ComputeEventHash(brainID, filePath string, lineOffset int64, entry *ParsedErrorLog) string {
	raw := fmt.Sprintf("%s|%s|%d|%s|%s|%d|%s",
		brainID, filePath, lineOffset, entry.Hostname, entry.ServiceName, entry.Timestamp.UnixNano(), entry.Message)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// -----------------------------------------------------------------------------

// ParseLogLine attempts to parse a standardized 8-column or cloud JSON log line.
// Returns (log, true) strictly for canonical ERROR (10) and CRITICAL (11) entries.
func ParseLogLine(line string) (*ParsedErrorLog, bool) {
	trimmed := strings.TrimSpace(sanitizeLogText(line))
	if len(trimmed) == 0 {
		return nil, false
	}

	// Containerized flexible-logger cloud profile writes JSONSerializer LogEntry lines.
	if trimmed[0] == '{' {
		return parseJSONLogLine(trimmed)
	}

	// Fast substring guard before column slicing or regex matching.
	if !strings.Contains(trimmed, "ERROR") && !strings.Contains(trimmed, "CRITICAL") {
		return nil, false
	}

	// Fixed-width columns: ts[0:33] host[34:46] svc[47:69] level[70:80] file[81:101] func[102:127] line[128:134] msg[135:]
	if len(trimmed) >= 135 {
		lvlRaw := strings.TrimSpace(trimmed[70:80])
		if lvlNum, ok := parseCanonicalLevel(lvlRaw); ok {
			ts, ok := parseLogTimestamp(strings.TrimSpace(trimmed[0:33]))
			if !ok {
				return nil, false
			}
			svc := strings.TrimSpace(trimmed[47:69])
			msgPart := strings.TrimSpace(trimmed[135:])
			return &ParsedErrorLog{
				Timestamp:    ts,
				Hostname:     strings.TrimSpace(trimmed[34:46]),
				ServiceName:  svc,
				LoggerName:   svc,
				Level:        lvlNum,
				LevelStr:     lvlRaw,
				Filename:     strings.TrimSpace(trimmed[81:101]),
				FunctionName: strings.TrimSpace(trimmed[102:127]),
				LineNumber:   strings.TrimSpace(trimmed[128:134]),
				Message:      msgPart,
				StackTrace:   extractStackTrace(msgPart),
			}, true
		}
	}

	// Fallback regex parsing for variably spaced lines.
	matches := fallbackLogRegex.FindStringSubmatch(trimmed)
	if len(matches) != 9 {
		return nil, false
	}
	lvlRaw := matches[4]
	lvlNum, ok := parseCanonicalLevel(lvlRaw)
	if !ok {
		return nil, false
	}
	ts, ok := parseLogTimestamp(matches[1])
	if !ok {
		return nil, false
	}
	msgPart := matches[8]
	return &ParsedErrorLog{
		Timestamp:    ts,
		Hostname:     matches[2],
		ServiceName:  matches[3],
		LoggerName:   matches[3],
		Level:        lvlNum,
		LevelStr:     lvlRaw,
		Filename:     matches[5],
		FunctionName: matches[6],
		LineNumber:   matches[7],
		Message:      msgPart,
		StackTrace:   extractStackTrace(msgPart),
	}, true
}

func parseJSONLogLine(trimmed string) (*ParsedErrorLog, bool) {
	// Fast substring guard before json.Unmarshal so 99%+ INFO/DEBUG/WARNING JSON lines skip decoding.
	if !strings.Contains(trimmed, ":10") && !strings.Contains(trimmed, ": 10") &&
		!strings.Contains(trimmed, ":11") && !strings.Contains(trimmed, ": 11") &&
		!strings.Contains(trimmed, `"ERROR"`) && !strings.Contains(trimmed, `"CRITICAL"`) {
		return nil, false
	}

	var raw struct {
		Timestamp    string          `json:"timestamp"`
		Level        json.RawMessage `json:"level"`
		Message      string          `json:"message"`
		Hostname     string          `json:"hostname"`
		LoggerName   string          `json:"loggerName"`
		Filename     string          `json:"filename"`
		FunctionName string          `json:"functionName"`
		LineNumber   string          `json:"lineNumber"`
		StackTrace   string          `json:"stackTrace"`
		ServiceName  string          `json:"serviceName"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, false
	}

	var lvlNum int
	var lvlStr string
	switch string(bytes.TrimSpace(raw.Level)) {
	case "10", `"ERROR"`:
		lvlNum = 10
		lvlStr = "ERROR"
	case "11", `"CRITICAL"`:
		lvlNum = 11
		lvlStr = "CRITICAL"
	default:
		return nil, false
	}

	ts, ok := parseLogTimestamp(raw.Timestamp)
	if !ok || raw.Message == "" {
		return nil, false
	}

	svc := raw.ServiceName
	if svc == "" {
		svc = raw.LoggerName
	}

	return &ParsedErrorLog{
		Timestamp:    ts,
		Hostname:     raw.Hostname,
		ServiceName:  svc,
		LoggerName:   raw.LoggerName,
		Level:        lvlNum,
		LevelStr:     lvlStr,
		Filename:     raw.Filename,
		FunctionName: raw.FunctionName,
		LineNumber:   raw.LineNumber,
		Message:      raw.Message,
		StackTrace:   raw.StackTrace,
	}, true
}
