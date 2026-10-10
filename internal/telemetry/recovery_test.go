package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/config"
)

func TestRecoverRecordsRedactedStackAndRequest(t *testing.T) {
	var output bytes.Buffer
	redactor := config.NewSecretRedactor()
	redactor.Replace(config.Config{ServerKeys: []string{"test-private-key"}})
	logger := NewStructuredLoggerWithOutput(&output, new(slog.LevelVar), NewLogHub(100), redactor)
	meta := &RequestMeta{TraceID: 42, Request: "req_test"}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req = req.WithContext(WithRequestMeta(req.Context(), meta))
	response := httptest.NewRecorder()
	Recover(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("unexpected test-private-key")
	})).ServeHTTP(response, req)
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if response.Code != 500 || meta.Outcome != "handler_panic" || record["request_id"] != "req_test" || record["trace_id"] != float64(42) {
		t.Fatalf("lost panic outcome or correlation: code=%d, meta=%+v, log=%v", response.Code, meta, record)
	}
	if !strings.Contains(record["stack"].(string), "recovery_test.go:") || bytes.Contains(output.Bytes(), []byte("test-private-key")) {
		t.Fatalf("missing stack or leaked secret: %s", output.Bytes())
	}
}

func TestRecoverPreservesHTTPAbortHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	defer func() {
		if value := recover(); value != http.ErrAbortHandler {
			t.Fatalf("expected net/http abort sentinel, got %v", value)
		}
	}()
	Recover(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRequestStartSurvivesUnfinishedRequest(t *testing.T) {
	var output bytes.Buffer
	logger := NewStructuredLoggerWithOutput(&output, new(slog.LevelVar), NewLogHub(100), config.NewSecretRedactor())
	monitor := NewMonitor()
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	meta := &RequestMeta{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("private conversation"))
	req = req.WithContext(WithRequestMeta(req.Context(), meta))
	handler := Middleware(monitor, logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-finish
		w.WriteHeader(http.StatusBadRequest)
	}))
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(finish)
		t.Fatal("request handler did not start")
	}
	initial := append([]byte(nil), output.Bytes()...)
	active, streams := monitor.Activity()
	trace := meta.TraceID
	close(finish)
	<-done
	if active != 1 || streams != 0 || trace == 0 || !bytes.Contains(initial, []byte("request_started")) || bytes.Contains(initial, []byte("request_complete")) {
		t.Fatalf("request was not visible while in flight: active=%d, streams=%d, trace=%d, log=%s", active, streams, trace, initial)
	}
	if !bytes.Contains(output.Bytes(), []byte("request_complete")) || bytes.Contains(output.Bytes(), []byte("private conversation")) {
		t.Fatalf("failed request missing or body unexpectedly logged: %s", output.Bytes())
	}
	active, _ = monitor.Activity()
	if active != 0 {
		t.Fatalf("request counter did not recover: %d", active)
	}
}

func TestRequestTraceDoesNotRepeatAcrossHandlers(t *testing.T) {
	monitor := NewMonitor()
	logger := NewStructuredLoggerWithOutput(&bytes.Buffer{}, new(slog.LevelVar), NewLogHub(100), config.NewSecretRedactor())
	var previous uint64
	for i := 0; i < 3; i++ {
		meta := &RequestMeta{}
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		req = req.WithContext(WithRequestMeta(req.Context(), meta))
		Middleware(monitor, logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), req)
		if meta.TraceID <= previous {
			t.Fatalf("trace repeated after rebuilding handler: previous=%d, current=%d", previous, meta.TraceID)
		}
		previous = meta.TraceID
	}
}
