package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
)

var (
	mu        sync.Mutex
	logFile   *os.File
	localFile *os.File
	logPath   string
)

type safeWriter struct {
	primary *os.File
	local   *os.File
}

func (s *safeWriter) Write(p []byte) (int, error) {
	_, _ = os.Stderr.Write(p)
	if s.local != nil {
		_, _ = s.local.Write(p)
	}
	return s.primary.Write(p)
}

// Init initializes application logging to a persistent log file and stderr.
func Init(customPath string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return logPath, nil
	}

	target := customPath
	isDefault := false
	if target == "" {
		isDefault = true
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = "."
		}
		target = filepath.Join(configDir, "com.github.mrpoundsign.hearsay", "logs", "hearsay.log")
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("open log file: %w", err)
	}

	var localF *os.File
	if isDefault {
		localPath, _ := filepath.Abs("hearsay.log")
		absTarget, _ := filepath.Abs(target)
		if localPath != absTarget {
			localF, _ = os.OpenFile("hearsay.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		}
	}

	logFile = f
	localFile = localF
	logPath = target

	log.SetOutput(&safeWriter{primary: f, local: localF})
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	return target, nil
}

// LogPath returns the currently active persistent log file path.
func LogPath() string {
	mu.Lock()
	p := logPath
	mu.Unlock()
	if p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "hearsay.log"
	}
	return filepath.Join(configDir, "com.github.mrpoundsign.hearsay", "logs", "hearsay.log")
}

// Sync flushes pending buffered log writes to disk.
func Sync() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Sync()
	}
	if localFile != nil {
		_ = localFile.Sync()
	}
}

// Close closes the active log file(s).
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
		logFile = nil
		logPath = ""
	}
	if localFile != nil {
		_ = localFile.Sync()
		_ = localFile.Close()
		localFile = nil
	}
}

// RecoverAndExit catches panics, writes a full stack trace to the log file,
// flushes the log, and exits with code 1.
func RecoverAndExit(context string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		log.Printf("FATAL PANIC [%s]: %v\nStack trace:\n%s", context, r, string(stack))
		Close()
		os.Exit(1)
	}
}
