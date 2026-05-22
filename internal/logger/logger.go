package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init initializes zerolog with pretty console output.
// Call this once at startup (e.g. in main.go).
func Init() {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	log.Logger = zerolog.New(output).
		With().
		Timestamp().
		Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

// InitWithLevel initializes zerolog with a specific level.
// level: "debug" | "info" | "warn" | "error"
func InitWithLevel(level string) {
	Init()
	SetLevel(level)
}

// SetLevel updates the global log level.
func SetLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
