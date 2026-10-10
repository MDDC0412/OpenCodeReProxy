package telemetry

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const runtimeLogMaxBytes = 10 << 20
const runtimeLogBackups = 3

// Diagnostics keeps each process's logs separate so a restart or another
// instance cannot overwrite the evidence of an earlier crash.
type Diagnostics struct {
	RunID       string
	RuntimePath string
	CrashPath   string
	CrashError  error
	runtime     *rotatingLog
	console     io.Writer
	warnOnce    sync.Once
}

func OpenDiagnostics(directory string) (*Diagnostics, error) {
	var err error
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	id := fmt.Sprintf("%s-%d", started.Format("20060102T150405.000000000Z"), os.Getpid())
	d := &Diagnostics{
		RunID: id, console: os.Stdout,
		RuntimePath: filepath.Join(directory, "runtime-"+id+".log"),
		CrashPath:   filepath.Join(directory, "crash-"+id+".log"),
	}
	d.runtime, err = newRotatingLog(d.RuntimePath, runtimeLogMaxBytes, runtimeLogBackups)
	if err != nil {
		return nil, err
	}
	d.CrashError = d.enableCrashOutput(started)
	return d, nil
}

// OpenConsoleReport is written by the surviving supervisor, including stderr
// emitted before Go's native crash-output hook starts copying fatal reports.
func OpenConsoleReport(directory string) (string, io.WriteCloser, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", nil, err
	}
	id := fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(directory, "console-"+id+".log")
	w, err := newRotatingLog(path, runtimeLogMaxBytes, runtimeLogBackups)
	if err != nil {
		return "", nil, err
	}
	return path, w, nil
}

func (d *Diagnostics) enableCrashOutput(started time.Time) error {
	f, err := os.OpenFile(d.CrashPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "OpenCode2API Go crash capture\nrun_id: %s\nstarted_at: %s\npid: %d\ngo: %s\nplatform: %s/%s\n\n", d.RunID, started.Format(time.RFC3339Nano), os.Getpid(), runtime.Version(), runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	// The runtime duplicates the descriptor and writes fatal errors directly.
	// This also captures panics in background goroutines and fatal errors that
	// recover cannot catch, without relying on deferred cleanup to run.
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

func (d *Diagnostics) Write(data []byte) (int, error) {
	_, diskErr := d.runtime.Write(data)
	if diskErr != nil {
		d.warnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "opencode2api: cannot write runtime log %s: %v\n", d.RuntimePath, diskErr)
		})
	}
	// Attempt both outputs even if one fails, for example when stdout is a
	// disconnected pipe. An unavailable console must not discard disk logs.
	n, consoleErr := d.console.Write(data)
	if diskErr == nil {
		n = len(data)
	}
	return n, errors.Join(diskErr, consoleErr)
}

func (d *Diagnostics) Close() error {
	// Do not disable SetCrashOutput here: main's deferred cleanup also runs
	// while unwinding a panic, before the runtime prints its fatal report.
	return d.runtime.Close()
}

type rotatingLog struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	maxBytes int64
	backups  int
	closed   bool
}

func newRotatingLog(path string, maxBytes int64, backups int) (*rotatingLog, error) {
	w := &rotatingLog{path: path, maxBytes: maxBytes, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingLog) open() error {
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file, w.size = f, info.Size()
	return nil
}

func (w *rotatingLog) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	return n, err
}

func (w *rotatingLog) rotate() (err error) {
	err = w.file.Close()
	w.file = nil
	// Always reopen after a partial rotation failure so later writes can
	// recover when a transient Windows file lock or disk error clears.
	defer func() { err = errors.Join(err, w.open()) }()
	if err != nil {
		return err
	}
	for i := w.backups; i >= 1; i-- {
		destination := fmt.Sprintf("%s.%d", w.path, i)
		if i == w.backups {
			if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		source := w.path
		if i > 1 {
			source = fmt.Sprintf("%s.%d", w.path, i-1)
		}
		if err := os.Rename(source, destination); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (w *rotatingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := errors.Join(w.file.Sync(), w.file.Close())
	w.file = nil
	return err
}
