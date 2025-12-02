package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// StatusCode represents domain-specific operation types
type StatusCode string

const (
	StatusInit   StatusCode = "INIT"   // Initialization/Discovery
	StatusOK     StatusCode = "OK"     // Success confirmation
	StatusErr    StatusCode = "ERR"    // Errors/failures
	StatusWarn   StatusCode = "WARN"   // Warnings/caution
	StatusData   StatusCode = "DATA"   // Economic data
	StatusGlob   StatusCode = "GLOB"   // International/web
	StatusLink   StatusCode = "LINK"   // Trade relationships
	StatusChk    StatusCode = "CHK"    // Validation/search
	StatusNat    StatusCode = "NAT"    // Nation entities
	StatusInd    StatusCode = "IND"    // Industries
	StatusCor    StatusCode = "COR"    // Corporations
	StatusMat    StatusCode = "MAT"    // Materials/commodities
	StatusRec    StatusCode = "REC"    // Recursive operations
	StatusNews   StatusCode = "NEWS"   // News monitoring
	StatusNew    StatusCode = "NEW"    // New discoveries
	StatusShock  StatusCode = "SHOCK"  // Shock events
	StatusHlth   StatusCode = "HLTH"   // Health metrics
	StatusRipple StatusCode = "RIPPLE" // Cascade effects
	StatusMon    StatusCode = "MON"    // Market monitoring
	StatusTag    StatusCode = "TAG"    // Metadata/tickers
	StatusFin    StatusCode = "$"      // Financial data
	StatusSoc    StatusCode = "SOC"    // Social media
	StatusSave   StatusCode = "SAVE"   // Persistence
	StatusWait   StatusCode = "WAIT"   // Rate limiting
	StatusTrend  StatusCode = "TREND↓" // Negative trends
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Logger handles all logging operations
type Logger struct {
	mu           sync.Mutex
	out          io.Writer
	level        LogLevel
	enableColors bool
	noColors     bool // Force disable colors (e.g., when piped)
}

var globalLogger *Logger
var once sync.Once

// Init initializes the global logger
func Init(level string, enableColors bool) {
	once.Do(func() {
		globalLogger = &Logger{
			out:          os.Stdout,
			level:        parseLevel(level),
			enableColors: enableColors,
			noColors:     !isTerminal() || os.Getenv("NO_COLOR") != "",
		}
	})
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		Init("info", true)
	}
	return globalLogger
}

// parseLevel converts string to LogLevel
func parseLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// isTerminal checks if output is a terminal (not piped)
func isTerminal() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// colorize applies ANSI color codes if enabled
func (l *Logger) colorize(color, text string) string {
	if l.enableColors && !l.noColors {
		return color + text + colorReset
	}
	return text
}

// getStatusColor returns the appropriate color for a status code
func (l *Logger) getStatusColor(status StatusCode) string {
	switch status {
	case StatusInit, StatusOK, StatusNew, StatusFin, StatusSave:
		return colorGreen
	case StatusErr, StatusShock, StatusTrend:
		return colorRed
	case StatusWarn, StatusWait:
		return colorYellow
	case StatusData, StatusNews, StatusChk, StatusMon, StatusSoc:
		return colorBlue
	case StatusGlob, StatusLink, StatusRec, StatusHlth, StatusRipple:
		return colorCyan
	default:
		return colorWhite
	}
}

// formatMessage builds the log message with timestamp and status
func (l *Logger) formatMessage(depth int, status StatusCode, format string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	var statusStr string
	if status != "" {
		statusStr = fmt.Sprintf("[%s] ", status)
		statusStr = l.colorize(l.getStatusColor(status), statusStr)
	}

	return fmt.Sprintf("%s %s%s", timestamp, statusStr, message)
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, depth int, status StatusCode, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := l.formatMessage(depth, status, format, args...)
	fmt.Fprintln(l.out, msg)
}

// Debug logs a debug message
func Debug(status StatusCode, format string, args ...interface{}) {
	GetLogger().log(DEBUG, 0, status, format, args...)
}

// DebugDepth logs a debug message with indentation
func DebugDepth(depth int, status StatusCode, format string, args ...interface{}) {
	GetLogger().log(DEBUG, depth, status, format, args...)
}

// Info logs an informational message
func Info(status StatusCode, format string, args ...interface{}) {
	GetLogger().log(INFO, 0, status, format, args...)
}

// InfoDepth logs an informational message with indentation
func InfoDepth(depth int, status StatusCode, format string, args ...interface{}) {
	GetLogger().log(INFO, depth, status, format, args...)
}

// Warn logs a warning message
func Warn(status StatusCode, format string, args ...interface{}) {
	GetLogger().log(WARN, 0, status, format, args...)
}

// WarnDepth logs a warning message with indentation
func WarnDepth(depth int, status StatusCode, format string, args ...interface{}) {
	GetLogger().log(WARN, depth, status, format, args...)
}

// Error logs an error message
func Error(status StatusCode, format string, args ...interface{}) {
	GetLogger().log(ERROR, 0, status, format, args...)
}

// ErrorDepth logs an error message with indentation
func ErrorDepth(depth int, status StatusCode, format string, args ...interface{}) {
	GetLogger().log(ERROR, depth, status, format, args...)
}

// Success logs a success message (always uses StatusOK)
func Success(format string, args ...interface{}) {
	GetLogger().log(INFO, 0, StatusOK, format, args...)
}

// SuccessDepth logs a success message with indentation
func SuccessDepth(depth int, format string, args ...interface{}) {
	GetLogger().log(INFO, depth, StatusOK, format, args...)
}

// Plain logs a message without status code or timestamp (for special formatting)
func Plain(format string, args ...interface{}) {
	l := GetLogger()
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.out, format+"\n", args...)
}

// Separator prints a visual separator line
func Separator() {
	Plain("==================================================")
}

// Section prints a section header
func Section(title string) {
	Separator()
	Plain("   %s", title)
	Separator()
}
