package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	TRACE LogLevel = iota
	DEBUG
	INFO
	WARN
	ERROR
)

func (l LogLevel) String() string {
	return [...]string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"}[l]
}

func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "TRACE":
		return TRACE
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

type Logger struct {
	level         LogLevel
	logger        *log.Logger
	file          *os.File
	filename      string
	retention     time.Duration
	mu            sync.Mutex
	cleanupTicker *time.Ticker
	stopCleanup   chan bool
}

type Config struct {
	Level     LogLevel
	Filename  string
	Retention time.Duration
}

func New(config Config) (*Logger, error) {
	// Ensure logs directory exists
	dir := filepath.Dir(config.Filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(config.Filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer to write to both file and stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	logger := &Logger{
		level:       config.Level,
		logger:      log.New(multiWriter, "", log.LstdFlags|log.Lmicroseconds),
		file:        file,
		filename:    config.Filename,
		retention:   config.Retention,
		stopCleanup: make(chan bool),
	}

	// Start cleanup routine
	logger.startCleanup()

	return logger, nil
}

func (l *Logger) Close() error {
	if l.cleanupTicker != nil {
		l.cleanupTicker.Stop()
		close(l.stopCleanup)
	}
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	prefix := fmt.Sprintf("[%s] ", level.String())
	message := fmt.Sprintf(format, args...)
	l.logger.Print(prefix + message)
}

func (l *Logger) Trace(format string, args ...interface{}) {
	l.log(TRACE, format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) startCleanup() {
	l.cleanupTicker = time.NewTicker(1 * time.Hour) // Check every hour

	go func() {
		for {
			select {
			case <-l.cleanupTicker.C:
				l.cleanupOldLogs()
			case <-l.stopCleanup:
				return
			}
		}
	}()
}

func (l *Logger) cleanupOldLogs() {
	if l.retention <= 0 {
		return // No cleanup if retention is not set
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-l.retention)

	// Get the directory of the log file
	dir := filepath.Dir(l.filename)
	baseName := filepath.Base(l.filename)

	// Remove extension to match rotated files
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	files, err := os.ReadDir(dir)
	if err != nil {
		l.logger.Printf("[ERROR] Failed to read log directory for cleanup: %v", err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if this is a log file for this logger (including rotated files)
		if !strings.HasPrefix(file.Name(), nameWithoutExt) {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		// Delete files older than retention period
		if info.ModTime().Before(cutoff) {
			oldFilePath := filepath.Join(dir, file.Name())
			if err := os.Remove(oldFilePath); err != nil {
				l.logger.Printf("[WARN] Failed to remove old log file %s: %v", oldFilePath, err)
			} else {
				l.logger.Printf("[INFO] Removed old log file: %s", oldFilePath)
			}
		}
	}
}

// ParseDuration parses duration strings like "3h", "24h", "7d"
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Handle common suffixes
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := time.ParseDuration(daysStr + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil // Convert days to hours (multiply by 24)
	}

	return time.ParseDuration(s)
}
