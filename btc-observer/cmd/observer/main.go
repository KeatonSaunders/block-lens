package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/keato/btc-observer/internal/database"
	"github.com/keato/btc-observer/internal/logger"
	"github.com/keato/btc-observer/internal/metrics"
	"github.com/keato/btc-observer/internal/observer"
)

func main() {
	logger.Log.Info().Msg("=== Bitcoin P2P Observer ===")
	logger.Log.Info().Msg("Network: MAINNET")
	logger.Log.Info().Msg("Regional peer selection enabled")

	// Load DB config and connect
	cfg, err := database.LoadConfig("config.json")
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("Failed to load config")
	}
	db, err := database.NewFromConfig(cfg)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	logger.Log.Info().Msg("Connected to database")

	// Seed Prometheus counters from historical DB totals
	metrics.SeedFromDB(db.Conn())

	// Start Prometheus metrics server
	metrics.StartMetricsServer(":9090")
	logger.Log.Info().Str("addr", ":9090").Msg("Prometheus metrics server started")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// WaitGroup to track active connections
	var wg sync.WaitGroup

	// Initialize peer manager
	pm := observer.NewPeerManager()

	// Start background routines
	observer.StartCleanupRoutine(ctx)

	// Initial peer discovery
	observer.RefreshPeerPool(pm)

	// Start periodic discovery (every 30 min)
	observer.StartDiscoveryRoutine(ctx, pm, 30*time.Minute)

	// Start peer manager (maintains connections)
	observer.StartPeerManager(ctx, pm, db, &wg)

	// Start status reporter
	observer.StartStatusReporter(ctx, pm, 60*time.Second)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Log.Info().Str("signal", sig.String()).Msg("Received signal, initiating graceful shutdown")

	// Cancel context to stop all goroutines
	cancel()

	// Close all active connections to unblock reads
	observer.CloseAllConnections()

	// Wait for all observer goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info().Msg("All connections closed gracefully")
	case <-time.After(10 * time.Second):
		logger.Log.Warn().Msg("Shutdown timeout - forcing exit")
	}

	// Close database connection
	if err := db.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("Error closing database")
	} else {
		logger.Log.Info().Msg("Database connection closed")
	}

	logger.Log.Info().Msg("Shutdown complete")
}
