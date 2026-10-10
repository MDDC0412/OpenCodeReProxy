package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"opencode2api/internal/buildinfo"
	"opencode2api/internal/telemetry"
)

type namedServer struct {
	server    *http.Server
	component string
}

type serverResult struct {
	component, address string
	err                error
}

func runServers(ctx context.Context, cancel context.CancelFunc, signals <-chan os.Signal, logger *slog.Logger, monitor *telemetry.Monitor, servers []namedServer) (int, string) {
	listeners := make([]net.Listener, 0, len(servers))
	for _, item := range servers {
		listener, err := net.Listen("tcp", item.server.Addr)
		if err != nil {
			for _, opened := range listeners {
				opened.Close()
			}
			cancel()
			logger.Error("server could not listen", "component", item.component, "event", "server_failed", "phase", "listen", "address", item.server.Addr, "error", err)
			return 1, "listener_failed"
		}
		listeners = append(listeners, listener)
	}
	results := make(chan serverResult, len(servers))
	for i, item := range servers {
		listener := listeners[i]
		logger.Info("server listening", "component", item.component, "event", "server_started", "address", listener.Addr().String(), "version", buildinfo.Version)
		go func() {
			results <- serverResult{component: item.component, address: listener.Addr().String(), err: item.server.Serve(listener)}
		}()
	}
	started := time.Now()
	logProcessHealth(logger, monitor, started)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	code, reason := 0, "context_canceled"
wait:
	for {
		select {
		case received := <-signals:
			reason = "signal"
			logger.Info("shutdown signal received", "component", "process", "event", "process_stopping", "reason", reason, "signal", received.String())
			break wait
		case result := <-results:
			code, reason = 1, "server_failed"
			logger.Error("server stopped unexpectedly", "component", result.component, "event", "server_failed", "phase", "serve", "address", result.address, "error", result.err)
			break wait
		case <-ctx.Done():
			logger.Info("shutdown context canceled", "component", "process", "event", "process_stopping", "reason", reason, "error", ctx.Err())
			break wait
		case <-ticker.C:
			logProcessHealth(logger, monitor, started)
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	for _, item := range servers {
		if err := item.server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "component", item.component, "event", "shutdown_failed", "address", item.server.Addr, "error", err)
			item.server.Close()
			code = 1
		}
	}
	logProcessHealth(logger, monitor, started)
	return code, reason
}

func logProcessHealth(logger *slog.Logger, monitor *telemetry.Monitor, started time.Time) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	requests, streams := monitor.Activity()
	logger.Info("process health", "component", "process", "event", "process_health", "uptime_seconds", int64(time.Since(started).Seconds()),
		"goroutines", runtime.NumGoroutine(), "heap_alloc_bytes", memory.HeapAlloc, "heap_inuse_bytes", memory.HeapInuse,
		"heap_sys_bytes", memory.HeapSys, "sys_bytes", memory.Sys, "gc_cycles", memory.NumGC, "active_requests", requests, "active_streams", streams)
}
