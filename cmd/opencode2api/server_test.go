package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/telemetry"
)

func TestListenerFailureDoesNotReportSuccess(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	code, reason := runServers(ctx, cancel, nil, logger, telemetry.NewMonitor(), []namedServer{
		{server: &http.Server{Addr: occupied.Addr().String()}, component: "api"},
	})
	if code != 1 || reason != "listener_failed" || ctx.Err() == nil {
		t.Fatalf("listener failure reported success: code=%d, reason=%s, ctx=%v", code, reason, ctx.Err())
	}
	if bytes.Contains(output.Bytes(), []byte("server_started")) || !bytes.Contains(output.Bytes(), []byte("server_failed")) {
		t.Fatalf("misleading startup log: %s", output.Bytes())
	}
}

func TestSecondListenerFailureReleasesFirst(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	firstAddress := first.Addr().String()
	first.Close()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	code, reason := runServers(ctx, cancel, nil, slog.New(slog.NewJSONHandler(&output, nil)), telemetry.NewMonitor(), []namedServer{
		{server: &http.Server{Addr: firstAddress}, component: "api"},
		{server: &http.Server{Addr: occupied.Addr().String()}, component: "webui"},
	})
	if code != 1 || reason != "listener_failed" {
		t.Fatalf("webui listener failure reported success: %d, %s", code, reason)
	}
	available, err := net.Listen("tcp", firstAddress)
	if err != nil {
		t.Fatalf("API listener leaked on WebUI failure: %v", err)
	}
	available.Close()
	if bytes.Contains(output.Bytes(), []byte("server_started")) {
		t.Fatalf("partial startup was reported as ready: %s", output.Bytes())
	}
}

func TestSignalShutdownRecordsReasonAndHealth(t *testing.T) {
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	code, reason := runServers(ctx, cancel, signals, slog.New(slog.NewJSONHandler(&output, nil)), telemetry.NewMonitor(), []namedServer{
		{server: &http.Server{Addr: "127.0.0.1:0"}, component: "api"},
	})
	if code != 0 || reason != "signal" {
		t.Fatalf("normal interrupt reported failure: %d, %s", code, reason)
	}
	if !bytes.Contains(output.Bytes(), []byte("process_stopping")) || !bytes.Contains(output.Bytes(), []byte("process_health")) {
		t.Fatalf("shutdown evidence missing: %s", output.Bytes())
	}
	dec := json.NewDecoder(&output)
	for {
		var record map[string]any
		if err := dec.Decode(&record); err != nil {
			break
		}
		if record["event"] == "server_started" && strings.HasSuffix(record["address"].(string), ":0") {
			t.Fatalf("recorded configured address instead of bound address: %v", record)
		}
	}
}

type startupHandler struct {
	slog.Handler
	started chan struct{}
}

func (h startupHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.Handler.Handle(ctx, record)
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "event" && attr.Value.String() == "server_started" {
			close(h.started)
		}
		return true
	})
	return err
}

func TestUnexpectedServerCloseReturnsFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	started := make(chan struct{})
	logger := slog.New(startupHandler{Handler: slog.NewJSONHandler(&output, nil), started: started})
	server := &http.Server{Addr: "127.0.0.1:0"}
	done := make(chan struct{})
	var code int
	var reason string
	go func() {
		defer close(done)
		code, reason = runServers(ctx, cancel, nil, logger, telemetry.NewMonitor(), []namedServer{{server: server, component: "api"}})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	server.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("unexpected close did not end the service")
	}
	if code != 1 || reason != "server_failed" || !bytes.Contains(output.Bytes(), []byte("server_failed")) {
		t.Fatalf("unexpected close reported success: %d, %s; %s", code, reason, output.Bytes())
	}
}

func TestStartupErrorIsPersisted(t *testing.T) {
	if directory := os.Getenv("O2API_STARTUP_TEST_DIRECTORY"); directory != "" {
		os.Exit(run([]string{"-config", filepath.Join(directory, "missing-config.json")}))
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartupErrorIsPersisted$")
	cmd.Env = append(os.Environ(), "O2API_STARTUP_TEST_DIRECTORY="+directory)
	output, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 1 || ctx.Err() != nil {
		t.Fatalf("startup error exit status: %v; %s", err, output)
	}
	files, _ := filepath.Glob(filepath.Join(directory, "logs", "runtime-*.log"))
	if len(files) != 1 {
		t.Fatalf("startup log missing: %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || !bytes.Contains(data, []byte("configuration_failed")) || !bytes.Contains(data, []byte("process_stopped")) {
		t.Fatalf("startup failure evidence lost: %v; %s", err, data)
	}
}
