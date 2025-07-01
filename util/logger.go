package util

import (
	"log/slog"
	"os"
	"time"

	stdlog "log"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	slogzerolog "github.com/samber/slog-zerolog/v2"
)

// LogLevel represents available log levels
type LogLevel = int

const (
	// Log levels
	DebugLevel LogLevel = iota
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
	if level == DebugLevel {
		ctx = ctx.Caller()
	}
	log.Logger = ctx.Logger()
	log.Info().Msg("Logger initialized")
}

// GetLogger returns a configured logger for a specific component
func GetLogger(component string) zerolog.Logger {
	return log.With().Str("component", component).Logger()
}

func NewSlogHandler(component string, lvl slog.Level) slog.Handler {
	opt := slogzerolog.Option{Level: lvl}

	zlog := log.With().Str("component", component).Logger()
	opt.Logger = &zlog

	return opt.NewZerologHandler()
}

func NewLogLogger(component string) *stdlog.Logger {
	zlvl := zerolog.GlobalLevel()
	var slvl slog.Level
	switch zlvl {
	case zerolog.DebugLevel:
		slvl = slog.LevelDebug
	case zerolog.InfoLevel:
		slvl = slog.LevelInfo
	case zerolog.WarnLevel:
		slvl = slog.LevelWarn
	case zerolog.ErrorLevel:
		slvl = slog.LevelError
	default:
		slvl = slog.LevelInfo
	}
	handler := NewSlogHandler(component, slvl)

	return slog.NewLogLogger(handler, slog.LevelInfo)
}
