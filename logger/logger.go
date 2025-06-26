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
	level              LogLevel
	logger             *log.Logger
	file               *os.File
	filename           string
	retention          time.Duration
	mu                 sync.Mutex
	cleanupTicker      *time.Ticker
	stopCleanup        chan bool
	lastLogTime        time.Time
	logDate            string
	healthCheckCounter int
	lastHealthLog      time.Time
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

	// Add date to filename to reduce redundancy in log lines
	now := time.Now()
	datedFilename := addDateToFilename(config.Filename, now)

	file, err := os.OpenFile(datedFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer to write to both file and stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	// Use shorter timestamp format (only time, not date)
	logger := &Logger{
		level:              config.Level,
		logger:             log.New(multiWriter, "", log.Ltime|log.Lmicroseconds),
		file:               file,
		filename:           datedFilename,
		retention:          config.Retention,
		stopCleanup:        make(chan bool),
		lastLogTime:        now,
		logDate:            now.Format("2006-01-02"),
		healthCheckCounter: 0,
		lastHealthLog:      time.Time{},
	}

	// Log startup message with date context
	logger.logger.Printf("[INFO] === LOG SESSION START %s ===", now.Format("2006-01-02 15:04:05"))

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

	// Smart content-based log level detection
	message := fmt.Sprintf(format, args...)
	adjustedLevel := l.detectLogLevel(message, level)

	// Health check summarization to reduce token usage
	if l.isHealthCheck(message) {
		if l.shouldSkipHealthLog() {
			return
		}
		message = l.summarizeHealthCheck(message)
	}

	// Remove redundant prefixes (INFO: INFO becomes just INFO)
	message = l.cleanMessage(message)

	prefix := fmt.Sprintf("[%s] ", adjustedLevel.String())
	l.logger.Print(prefix + message)
	l.lastLogTime = time.Now()
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

// addDateToFilename adds current date to filename for organization
func addDateToFilename(filename string, t time.Time) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	date := t.Format("2006-01-02")
	return fmt.Sprintf("%s-%s%s", base, date, ext)
}

// detectLogLevel analyzes message content to determine appropriate log level
func (l *Logger) detectLogLevel(message string, originalLevel LogLevel) LogLevel {
	lowerMsg := strings.ToLower(message)

	// Detect error content in INFO logs
	if originalLevel == INFO && (strings.Contains(lowerMsg, "error:") ||
		strings.Contains(lowerMsg, "failed") ||
		strings.Contains(lowerMsg, "timeout") ||
		strings.Contains(lowerMsg, "cancelled") ||
		strings.Contains(lowerMsg, "context deadline exceeded")) {
		return ERROR
	}

	// Detect warn content in INFO logs
	if originalLevel == INFO && (strings.Contains(lowerMsg, "warn") ||
		strings.Contains(lowerMsg, "deprecated") ||
		strings.Contains(lowerMsg, "method not found")) {
		return WARN
	}

	return originalLevel
}

// isHealthCheck determines if a message is related to health checking
func (l *Logger) isHealthCheck(message string) bool {
	return strings.Contains(message, "health_check") ||
		strings.Contains(message, "ping") ||
		strings.Contains(message, "Health check summary") ||
		strings.Contains(message, ">>> ") || // Simplified MCP communication markers
		strings.Contains(message, "<<< ") ||
		(strings.Contains(message, "=== MCP SEND AND RECEIVE") && strings.Contains(message, "health"))
}

// shouldSkipHealthLog implements health check log summarization
func (l *Logger) shouldSkipHealthLog() bool {
	now := time.Now()

	// Skip detailed health logs if we've seen too many recently
	if now.Sub(l.lastHealthLog) < 10*time.Minute {
		l.healthCheckCounter++
		// After first 2 health checks in 10 minutes, only log every 50th
		if l.healthCheckCounter > 2 && l.healthCheckCounter%50 != 0 {
			return true
		}
	} else {
		// Reset counter after 10 minutes of silence
		l.healthCheckCounter = 0
	}

	l.lastHealthLog = now
	return false
}

// summarizeHealthCheck creates concise health check summaries
func (l *Logger) summarizeHealthCheck(message string) string {
	if l.healthCheckCounter > 3 {
		return fmt.Sprintf("Health check summary: %d checks completed (latest: %s)",
			l.healthCheckCounter, time.Now().Format("15:04:05"))
	}
	return message
}

// cleanMessage removes redundant prefixes and cleans up formatting
func (l *Logger) cleanMessage(message string) string {
	// Remove redundant log level prefixes that appear after proper level detection
	message = strings.ReplaceAll(message, "INFO: INFO", "")
	message = strings.ReplaceAll(message, "ERROR: INFO", "")
	message = strings.ReplaceAll(message, "WARN: INFO", "")
	message = strings.ReplaceAll(message, "DEBUG: INFO", "")
	message = strings.ReplaceAll(message, "INFO: ERROR", "")
	message = strings.ReplaceAll(message, "ERROR: ERROR", "")
	message = strings.ReplaceAll(message, "WARN: WARN", "")
	message = strings.ReplaceAll(message, "DEBUG: DEBUG", "")

	// Remove redundant "INFO: " prefixes at start of message
	if strings.HasPrefix(message, "INFO: ") {
		message = strings.TrimPrefix(message, "INFO: ")
	}
	if strings.HasPrefix(message, "ERROR: ") {
		message = strings.TrimPrefix(message, "ERROR: ")
	}
	if strings.HasPrefix(message, "WARN: ") {
		message = strings.TrimPrefix(message, "WARN: ")
	}
	if strings.HasPrefix(message, "DEBUG: ") {
		message = strings.TrimPrefix(message, "DEBUG: ")
	}

	// Simplify repetitive boundary markers
	message = strings.ReplaceAll(message, "=== MCP SEND AND RECEIVE START (Server: ", ">>> ")
	message = strings.ReplaceAll(message, "=== MCP SEND AND RECEIVE END (Server: ", "<<< ")
	message = strings.ReplaceAll(message, ") - SUCCESS ===", " OK")
	message = strings.ReplaceAll(message, ") ===", "")

	// Truncate very long JSON in messages (keep first 100 chars)
	if len(message) > 200 && strings.Contains(message, "{") {
		if idx := strings.Index(message, "{"); idx != -1 {
			jsonPart := message[idx:]
			if len(jsonPart) > 100 {
				message = message[:idx] + jsonPart[:100] + "...}"
			}
		}
	}

	// Trim leading/trailing whitespace after cleanup
	message = strings.TrimSpace(message)

	return message
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
