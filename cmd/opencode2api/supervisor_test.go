package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/telemetry"
)

func TestSupervisorCapturesChildFailure(t *testing.T) {
	if mode := os.Getenv("O2API_SUPERVISOR_TEST_MODE"); mode != "" {
		directory := os.Getenv("O2API_SUPERVISOR_TEST_DIRECTORY")
		if mode == "configuration" {
			// always would recurse without the internal child marker.
			os.Exit(run([]string{"-config", filepath.Join(directory, "missing.json"), "-pause-on-error", "always"}))
		}
		d, err := telemetry.OpenDiagnostics(directory)
		if err != nil || d.CrashError != nil {
			t.Fatalf("crash initialization failed: %v, %+v", err, d)
		}
		defer d.Close()
		debug.SetTraceback("all")
		debug.SetMaxStack(64 << 10)
		supervisorStackOverflow(1)
		return
	}
	for _, mode := range []string{"configuration", "fatal"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSupervisorCapturesChildFailure$")
			command.Env = append(os.Environ(), consoleChildEnvironment+"=1", "O2API_SUPERVISOR_TEST_MODE="+mode, "O2API_SUPERVISOR_TEST_DIRECTORY="+directory)
			path, report, err := telemetry.OpenConsoleReport(directory)
			if err != nil {
				t.Fatal(err)
			}
			code, cause := superviseCommand(command, report, io.Discard)
			report.Close()
			if ctx.Err() != nil || code == 0 {
				t.Fatalf("child failed to exit: code=%d, cause=%s, ctx=%v", code, cause, ctx.Err())
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("exit_code:")) || !bytes.Contains(data, []byte("service_pid:")) {
				t.Fatalf("exit details missing: %s", data)
			}
			if mode == "fatal" {
				if cause != "fatal error: stack overflow" || !bytes.Contains(data, []byte(cause)) || !bytes.Contains(data, []byte("supervisor_test.go:")) {
					t.Fatalf("full fatal cause or stack missing: cause=%s, log bytes=%d", cause, len(data))
				}
			} else if code != 1 || !strings.Contains(cause, "configuration error") || !bytes.Contains(data, []byte("configuration_failed")) {
				t.Fatalf("startup failure incorrectly explained: code=%d, cause=%s, log=%s", code, cause, data)
			}
		})
	}
}

//go:noinline
func supervisorStackOverflow(n int) int {
	return supervisorStackOverflow(n+1) + n
}

func TestWaitForEnterKeepsConsoleUntilInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	prompt := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		waitForEnter(reader, promptOutput{prompt: prompt})
	}()
	select {
	case <-prompt:
	case <-time.After(5 * time.Second):
		t.Fatal("pause prompt missing")
	}
	select {
	case <-done:
		t.Fatal("console exited before Enter")
	default:
	}
	io.WriteString(writer, "\n")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Enter did not release the console")
	}
}

type promptOutput struct{ prompt chan struct{} }

func (w promptOutput) Write(data []byte) (int, error) {
	w.prompt <- struct{}{}
	return len(data), nil
}

func TestSupervisorParserHandlesSplitOutputWithoutUnboundedBuffer(t *testing.T) {
	evidence := &exitEvidence{}
	writer := &supervisorOutput{console: io.Discard, report: io.Discard, evidence: evidence, stderr: true}
	writer.Write([]byte("fat"))
	writer.Write([]byte("al error: test fault\r\nstack follows\n"))
	if cause := evidence.cause(); cause != "fatal error: test fault" {
		t.Fatalf("lost split fatal cause: %s", cause)
	}
	writer.Write(bytes.Repeat([]byte("a"), 256<<10))
	if len(writer.pending) > 128<<10 {
		t.Fatalf("unbounded supervisor buffer: %d", len(writer.pending))
	}
}
