package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func init() {
	// Pretty console output for development
	// For production JSON, remove ConsoleWriter and use: zerolog.New(os.Stdout)
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	Log = zerolog.New(output).
		With().
		Timestamp().
		Logger()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

// SetJSONOutput switches to JSON logging (for production)
func SetJSONOutput() {
	Log = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()
}

// SetDebugLevel enables debug logging
func SetDebugLevel() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

// PeerLogger returns a logger with peer context
func PeerLogger(region, addr string) zerolog.Logger {
	return Log.With().
		Str("region", region).
		Str("peer", addr).
		Logger()
}
