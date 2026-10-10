package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"opencode2api/internal/config"
)

func TestRotatingLogPreservesWholeRecordsAndCapsBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	w, err := newRotatingLog(path, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"one\n", "two\n", "three\n", "four\n", "five\n"} {
		if _, err := w.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for suffix, want := range map[string]string{"": "five\n", ".1": "four\n", ".2": "three\n"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil || string(data) != want {
			t.Fatalf("%s: got %q, %v; want %q", suffix, data, err, want)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected third backup: %v", err)
	}
	if _, err := w.Write([]byte("closed\n")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
}

func TestRotatingLogConcurrentWritersKeepValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	w, err := newRotatingLog(path, 1<<20, 3)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Separate handlers have separate locks and share only this writer.
			logger := slog.New(slog.NewJSONHandler(w, nil))
			for j := 0; j < 100; j++ {
				logger.Info("concurrent record", "writer", i, "record", j)
			}
		}()
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	seen := make(map[string]bool)
	for {
		var record map[string]any
		if err := dec.Decode(&record); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		seen[fmt.Sprint(record["writer"], ":", record["record"])] = true
	}
	if len(seen) != 800 {
		t.Fatalf("lost or duplicated records: %d", len(seen))
	}
}

type unavailableLogOutput struct{}

func (unavailableLogOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDiagnosticsKeepsDiskAndHubWhenConsoleFails(t *testing.T) {
	d, err := OpenDiagnostics(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		d.Close()
	})
	if d.CrashError != nil {
		t.Fatal(d.CrashError)
	}
	d.console = unavailableLogOutput{}
	hub := NewLogHub(100)
	redactor := config.NewSecretRedactor()
	redactor.Replace(config.Config{ServerKeys: []string{"test-private-key"}})
	logger := NewStructuredLoggerWithOutput(d, new(slog.LevelVar), hub, redactor)
	logger.Error("cannot send request", "error", errors.New("test-private-key is invalid"))
	data, err := os.ReadFile(d.RuntimePath)
	if err != nil || !bytes.Contains(data, []byte("cannot send request")) || bytes.Contains(data, []byte("test-private-key")) {
		t.Fatalf("disk log missing or not redacted: %s, %v", data, err)
	}
	recent, _ := hub.Recent(0, 100)
	if len(recent) != 1 || strings.Contains(fmt.Sprint(recent[0].Fields), "test-private-key") {
		t.Fatalf("hub log missing or not redacted: %+v", recent)
	}
}

func TestStructuredLoggerPublishesWhenOutputFails(t *testing.T) {
	hub := NewLogHub(100)
	logger := NewStructuredLoggerWithOutput(unavailableLogOutput{}, new(slog.LevelVar), hub, config.NewSecretRedactor())
	logger.Info("still visible in the WebUI")
	recent, _ := hub.Recent(0, 100)
	if len(recent) != 1 || recent[0].Message != "still visible in the WebUI" {
		t.Fatalf("log lost when output failed: %+v", recent)
	}
}

func TestDiagnosticsNativeCrash(t *testing.T) {
	if mode := os.Getenv("O2API_DIAGNOSTIC_TEST_MODE"); mode != "" {
		d, err := OpenDiagnostics(os.Getenv("O2API_DIAGNOSTIC_TEST_DIRECTORY"))
		if err != nil || d.CrashError != nil {
			t.Fatalf("initialize crash output: %v, %+v", err, d)
		}
		d.console = io.Discard
		defer d.Close()
		debug.SetTraceback("all")
		logger := NewStructuredLoggerWithOutput(d, new(slog.LevelVar), NewLogHub(100), config.NewSecretRedactor())
		logger.Info("request in flight", "event", "request_started", "request_id", "diagnostic-test-request")
		switch mode {
		case "background":
			go func() { panic("diagnostic background panic") }()
			select {}
		case "main":
			panic("diagnostic main panic")
		case "fatal":
			debug.SetMaxStack(64 << 10)
			diagnosticStackOverflow(1)
		}
		return
	}
	for mode, marker := range map[string]string{
		"background": "panic: diagnostic background panic",
		"main":       "panic: diagnostic main panic",
		"fatal":      "runtime.newstack",
	} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDiagnosticsNativeCrash$")
			cmd.Env = append(os.Environ(), "O2API_DIAGNOSTIC_TEST_MODE="+mode, "O2API_DIAGNOSTIC_TEST_DIRECTORY="+directory)
			output, err := cmd.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || ctx.Err() != nil {
				t.Fatalf("child did not crash as expected: %v; %s", err, output)
			}
			if mode == "fatal" && !bytes.Contains(output, []byte("fatal error: stack overflow")) {
				t.Fatalf("child did not produce the expected fatal reason: %s", output)
			}
			files, _ := filepath.Glob(filepath.Join(directory, "crash-*.log"))
			if len(files) != 1 {
				t.Fatalf("expected one crash log, got %v", files)
			}
			data, err := os.ReadFile(files[0])
			if err != nil || !bytes.Contains(data, []byte(marker)) || !bytes.Contains(data, []byte("diagnostics_test.go:")) {
				t.Fatalf("missing fatal report or source location: %v; %s", err, data)
			}
			files, _ = filepath.Glob(filepath.Join(directory, "runtime-*.log"))
			if len(files) != 1 {
				t.Fatalf("expected one runtime log, got %v", files)
			}
			data, err = os.ReadFile(files[0])
			if err != nil || !bytes.Contains(data, []byte("diagnostic-test-request")) {
				t.Fatalf("in-flight request log lost on crash: %v; %s", err, data)
			}
		})
	}
}

//go:noinline
func diagnosticStackOverflow(n int) int {
	return diagnosticStackOverflow(n+1) + n
}
