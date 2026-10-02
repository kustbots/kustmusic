package ntgcalls

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents the severity level of a log message.
type LogLevel int

const (
	// LevelDebug enables debug level logging.
	LevelDebug LogLevel = iota
	// LevelInfo enables info level logging.
	LevelInfo
	// LevelWarn enables warning level logging.
	LevelWarn
	// LevelError enables error level logging.
	LevelError
	// LevelFatal enables fatal level logging.
	LevelFatal
)

// Logger provides structured logging for the ntgcalls package.
type Logger struct {
	name  string
	level LogLevel
	l     *log.Logger
}

// NewLogger creates a new Logger instance with the specified name and log level.
func NewLogger(name string, level LogLevel) *Logger {
	return &Logger{
		name:  name,
		level: level,
		l:     log.New(os.Stdout, fmt.Sprintf("[%s] ", name), log.LstdFlags),
	}
}

func (lg *Logger) log(level LogLevel, tag string, msg string) {
	if level >= lg.level {
		lg.l.Printf("[%s] %s", tag, msg)
	}
}

// Debug logs a debug message.
func (lg *Logger) Debug(msg string) { lg.log(LevelDebug, "DEBUG", msg) }

// Info logs an informational message.
func (lg *Logger) Info(msg string) { lg.log(LevelInfo, "INFO", msg) }

// Warn logs a warning message.
func (lg *Logger) Warn(msg string) { lg.log(LevelWarn, "WARN", msg) }

// Error logs an error message.
func (lg *Logger) Error(msg string) { lg.log(LevelError, "ERROR", msg) }

// Fatal logs a fatal error message.
func (lg *Logger) Fatal(msg string) { lg.log(LevelFatal, "FATAL", msg) }
