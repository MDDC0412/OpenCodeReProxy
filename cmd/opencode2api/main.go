// Command opencode2api starts the API gateway and its management interface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	adminui "opencode2api/internal/admin"
	"opencode2api/internal/buildinfo"
	"opencode2api/internal/config"
	"opencode2api/internal/gateway"
	"opencode2api/internal/telemetry"
)

// version remains the linker injection point used by release builds.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	buildinfo.Version = version
	flags := flag.NewFlagSet("opencode2api", flag.ContinueOnError)
	configPath := flags.String("config", "config.json", "path to config.json")
	listen := flags.String("listen", "", "override the configured API listen address")
	webListen := flags.String("web-listen", "", "override the configured WebUI listen address")
	logDir := flags.String("log-dir", "", "diagnostic log directory (default: logs beside config.json)")
	pauseMode := flags.String("pause-on-error", "auto", "keep console after a failed exit: auto (Windows double-click), always, never")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *pauseMode != "auto" && *pauseMode != "always" && *pauseMode != "never" {
		fmt.Fprintln(os.Stderr, "opencode2api: -pause-on-error must be auto, always, or never")
		return 2
	}
	absoluteConfig, err := filepath.Abs(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opencode2api: resolve configuration path:", err)
		return 1
	}
	if *logDir == "" {
		*logDir = filepath.Join(filepath.Dir(absoluteConfig), "logs")
	}
	if os.Getenv(consoleChildEnvironment) != "1" && (*pauseMode == "always" || *pauseMode == "auto" && ownsConsole()) {
		return runSupervised(args, *logDir, absoluteConfig)
	}
	debug.SetTraceback("all")
	diagnostics, logErr := openDiagnostics(*logDir, absoluteConfig)
	var output io.Writer = os.Stdout
	if diagnostics != nil {
		output = diagnostics
		defer func() {
			if err := diagnostics.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "opencode2api: close runtime log:", err)
			}
		}()
	}
	level := new(slog.LevelVar)
	hub := telemetry.NewLogHub(2000)
	redactor := config.NewSecretRedactor()
	logger := telemetry.NewStructuredLoggerWithOutput(output, level, hub, redactor).With("pid", os.Getpid())
	// Lifecycle evidence remains available even if business logs are configured
	// as warn/error or the WebUI changes the runtime log level.
	lifecycle := telemetry.NewStructuredLoggerWithOutput(output, new(slog.LevelVar), hub, redactor).With("pid", os.Getpid())
	if diagnostics != nil {
		logger = logger.With("run_id", diagnostics.RunID)
		lifecycle = lifecycle.With("run_id", diagnostics.RunID)
		lifecycle.Info("diagnostic files opened", "component", "process", "event", "diagnostics_started",
			"runtime_log", diagnostics.RuntimePath, "crash_log", diagnostics.CrashPath)
		if diagnostics.CrashError != nil {
			lifecycle.Warn("native crash capture unavailable", "component", "process", "event", "crash_capture_failed", "error", diagnostics.CrashError)
		}
	}
	if logErr != nil {
		lifecycle.Warn("preferred log directory unavailable", "component", "process", "event", "log_directory_failed", "directory", *logDir, "error", logErr)
	}
	executable, _ := os.Executable()
	workingDirectory, _ := os.Getwd()
	lifecycle.Info("process started", "component", "process", "event", "process_started", "version", buildinfo.Version,
		"go_version", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH,
		"executable", executable, "working_directory", workingDirectory, "config", absoluteConfig, "parent_pid", os.Getppid())

	cfg, err := config.Load(*configPath)
	if err != nil {
		lifecycle.Error("configuration error", "component", "process", "event", "configuration_failed", "error", err)
		lifecycle.Error("process stopped", "component", "process", "event", "process_stopped", "reason", "configuration_failed", "exit_code", 1)
		return 1
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *webListen != "" {
		cfg.WebUI.Listen = *webListen
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	telemetry.SetLogLevel(level, cfg.Logging.Level)
	hub.Resize(cfg.Logging.RingSize)
	redactor.Replace(cfg)
	monitor := telemetry.NewMonitor()
	manager, err := gateway.NewRuntimeManager(ctx, *configPath, cfg, logger, monitor, hub, redactor, level)
	if err != nil {
		lifecycle.Error("failed to initialize runtime", "component", "runtime", "event", "runtime_initialization_failed", "error", err)
		lifecycle.Error("process stopped", "component", "process", "event", "process_stopped", "reason", "runtime_initialization_failed", "exit_code", 1)
		return 1
	}
	defer manager.Shutdown()

	apiServer := &http.Server{
		Addr: cfg.Listen, Handler: manager.Handler(), ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second,
		ErrorLog: slog.NewLogLogger(logger.With("component", "api", "event", "http_server_error").Handler(), slog.LevelError),
	}
	servers := []namedServer{{server: apiServer, component: "api"}}

	if cfg.WebUI.Enabled {
		admin := adminui.New(manager, monitor, hub, logger)
		webServer := &http.Server{
			Addr: cfg.WebUI.Listen, Handler: admin.Handler(), ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second,
			ErrorLog: slog.NewLogLogger(logger.With("component", "webui", "event", "http_server_error").Handler(), slog.LevelError),
		}
		servers = append(servers, namedServer{server: webServer, component: "webui"})
	}

	code, reason := runServers(ctx, cancel, signals, lifecycle, monitor, servers)
	manager.Shutdown()
	lifecycle.Log(context.Background(), exitLevel(code), "process stopped", "component", "process", "event", "process_stopped", "reason", reason, "exit_code", code)
	return code
}

func openDiagnostics(directory, configPath string) (*telemetry.Diagnostics, error) {
	diagnostics, err := telemetry.OpenDiagnostics(directory)
	if err == nil {
		return diagnostics, nil
	}
	cache, cacheErr := os.UserCacheDir()
	if cacheErr != nil {
		return nil, errors.Join(err, cacheErr)
	}
	fallback := filepath.Join(cache, "opencode2api", "logs", config.Fingerprint(configPath))
	diagnostics, fallbackErr := telemetry.OpenDiagnostics(fallback)
	return diagnostics, errors.Join(err, fallbackErr)
}

func exitLevel(code int) slog.Level {
	if code != 0 {
		return slog.LevelError
	}
	return slog.LevelInfo
}
