package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/telemetry"
)

const consoleChildEnvironment = "O2API_CONSOLE_CHILD"

func runSupervised(args []string, directory, configPath string) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stdout, "OpenCode2API: cannot locate service executable:", err)
		waitForEnter(os.Stdin, os.Stdout)
		return 1
	}
	path, file, err := telemetry.OpenConsoleReport(directory)
	if err != nil {
		if cache, cacheErr := os.UserCacheDir(); cacheErr == nil {
			path, file, err = telemetry.OpenConsoleReport(filepath.Join(cache, "opencode2api", "logs", config.Fingerprint(configPath)))
		}
	}
	var report io.Writer = io.Discard
	if err != nil {
		fmt.Fprintln(os.Stdout, "OpenCode2API: cannot create console report:", err)
	} else {
		report = file
		defer file.Close()
	}
	command := exec.Command(executable, args...)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, consoleChildEnvironment) {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, consoleChildEnvironment+"=1")
	// Parent and child share the existing console. Keep the parent alive while
	// the child handles Ctrl+C and writes its graceful-shutdown evidence.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	code, cause := superviseCommand(command, report, os.Stdout)
	signal.Stop(signals)
	if file != nil {
		if err := file.Close(); err != nil {
			fmt.Fprintln(os.Stdout, "OpenCode2API: close console report:", err)
		}
	}
	if code != 0 {
		fmt.Fprintf(os.Stdout, "\n[OpenCode2API] 服务异常退出，退出码 %d (0x%08X)。\n最后记录的原因：%s\n", code, uint32(code), cause)
		if path != "" {
			fmt.Fprintf(os.Stdout, "控制台记录：%s\n运行日志和崩溃堆栈位于同一目录。\n", path)
		}
		waitForEnter(os.Stdin, os.Stdout)
	}
	return code
}

func waitForEnter(input io.Reader, output io.Writer) {
	fmt.Fprint(output, "窗口已保留，按回车键关闭……")
	_, _ = bufio.NewReader(input).ReadString('\n')
}

func superviseCommand(command *exec.Cmd, report, console io.Writer) (int, string) {
	evidence := &exitEvidence{}
	command.Stdout = &supervisorOutput{console: console, report: report, evidence: evidence}
	command.Stderr = &supervisorOutput{console: console, report: report, evidence: evidence, stderr: true}
	fmt.Fprintf(report, "OpenCode2API console supervisor\nstarted_at: %s\nsupervisor_pid: %d\nexecutable: %s\n\n", time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), command.Path)
	if err := command.Start(); err != nil {
		cause := "子服务启动失败：" + err.Error()
		fmt.Fprintln(report, cause)
		return 1, cause
	}
	fmt.Fprintf(report, "service_pid: %d\n", command.Process.Pid)
	err := command.Wait()
	code := 0
	if err != nil {
		code = 1
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			code = exited.ExitCode()
		}
	}
	cause := evidence.cause()
	fmt.Fprintf(report, "\nexited_at: %s\nexit_code: %d (0x%08X)\nlast_reason: %s\n", time.Now().UTC().Format(time.RFC3339Nano), code, uint32(code), cause)
	return code, cause
}

type exitEvidence struct {
	mu            sync.Mutex
	fatal, reason string
}

func (e *exitEvidence) cause() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fatal != "" {
		return e.fatal
	}
	if e.reason != "" {
		return e.reason
	}
	return "服务未报告退出原因；请检查日志，进程被外部强制终止时可能没有 Go 崩溃堆栈"
}

type supervisorOutput struct {
	console, report io.Writer
	evidence        *exitEvidence
	stderr          bool
	pending         []byte
	dropping        bool
}

func (w *supervisorOutput) Write(data []byte) (int, error) {
	// Each os/exec copy goroutine owns one output's framing buffer. The shared
	// evidence and report writer are synchronized separately.
	if w.stderr {
		_, _ = w.report.Write(data)
	}
	_, _ = w.console.Write(data)
	w.pending = append(w.pending, data...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		if !w.dropping {
			w.observeLine(strings.TrimSuffix(string(w.pending[:index]), "\r"))
		}
		w.dropping = false
		w.pending = w.pending[index+1:]
	}
	// A huge panic message or debug record must not make the supervisor retain
	// an unbounded line. The complete stderr still goes straight to disk.
	if len(w.pending) > 128<<10 {
		w.pending = nil
		w.dropping = true
	}
	// A failed console or log destination must not stop draining the child's
	// pipes and deadlock Wait while the child is printing a fatal report.
	return len(data), nil
}

func (w *supervisorOutput) observeLine(line string) {
	if w.stderr {
		if strings.HasPrefix(line, "panic:") || strings.HasPrefix(line, "fatal error:") || strings.HasPrefix(line, "Exception ") {
			w.evidence.mu.Lock()
			if w.evidence.fatal == "" {
				w.evidence.fatal = line
			}
			w.evidence.mu.Unlock()
		}
		return
	}
	var record struct {
		Level   string `json:"level"`
		Event   string `json:"event"`
		Error   string `json:"error"`
		Reason  string `json:"reason"`
		Message string `json:"msg"`
	}
	if json.Unmarshal([]byte(line), &record) != nil {
		return
	}
	if record.Level == "ERROR" || record.Event == "process_started" || record.Event == "diagnostics_started" || record.Event == "process_stopped" {
		fmt.Fprintln(w.report, line)
	}
	if record.Error != "" && record.Level == "ERROR" || record.Event == "process_stopped" {
		w.evidence.mu.Lock()
		if record.Error != "" {
			w.evidence.reason = record.Message + ": " + record.Error
		} else if w.evidence.reason == "" {
			w.evidence.reason = record.Reason
		}
		w.evidence.mu.Unlock()
	}
}
