package util

import (
	"os"
	"strings"
	"time"

	stdlog "log"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Logger = zerolog.Logger

// LogLevel represents available log levels
type LogLevel = int

// Log levels
const (
	TraceLevel LogLevel = iota
	DebugLevel
	InfoLevel
	WarnLevel
	ErrorLevel
)

// InitializeLogger sets up the global logger with the specified configuration
func InitializeLogger(level LogLevel) {
	// Set time format to ISO8601
	zerolog.TimeFieldFormat = time.RFC3339

	// Set global log level based on configuration
	switch level {
	case TraceLevel:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case DebugLevel:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case InfoLevel:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case WarnLevel:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case ErrorLevel:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Create a console writer with nice formatting for terminal output
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}

	// Set global logger
	ctx := zerolog.New(output).With().Timestamp()
	if level == TraceLevel {
		ctx = ctx.Caller()
	}
	log.Logger = ctx.Logger()
	log.Info().Msg("Logger initialized")
}

// GetLogger returns a configured logger for a specific component
func GetLogger(component string) zerolog.Logger {
	return log.With().Str("component", component).Logger()
}

// zerologWriter wraps zerolog to implement io.Writer for stdlog
type zerologWriter struct {
	logger zerolog.Logger
	level  zerolog.Level
}

func (w zerologWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	// Remove stdlog prefix if present (timestamp and flags)
	if idx := strings.LastIndex(msg, ": "); idx != -1 && idx < len(msg)-2 {
		msg = msg[idx+2:]
	}
	w.logger.WithLevel(w.level).Msg(msg)

	return len(p), nil
}

// NewLogLogger returns a configured stdlog.Logger that routes to zerolog
// TODO: this Writer technique doesn't pass down context i.e. call location
func NewLogLogger(component string, lvl LogLevel) *stdlog.Logger {
	var zerologLevel zerolog.Level
	switch lvl {
	case TraceLevel:
		zerologLevel = zerolog.TraceLevel
	case DebugLevel:
		zerologLevel = zerolog.DebugLevel
	case InfoLevel:
		zerologLevel = zerolog.InfoLevel
	case WarnLevel:
		zerologLevel = zerolog.WarnLevel
	case ErrorLevel:
		zerologLevel = zerolog.ErrorLevel
	default:
		zerologLevel = zerolog.InfoLevel
	}

	logger := log.With().Str("component", component).Logger()
	writer := zerologWriter{logger: logger, level: zerologLevel}

	return stdlog.New(writer, "", 0)
}
