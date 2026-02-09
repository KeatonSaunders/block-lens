package metrics

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Transaction metrics
	TxReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_transactions_received_total",
		Help: "Total number of transactions received",
	})

	TxRecordedDB = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_transactions_recorded_total",
		Help: "Total number of transactions recorded to database",
	})

	TxConflicts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_transaction_conflicts_total",
		Help: "Total number of double-spend conflicts detected",
	})

	// Block metrics
	BlocksReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_blocks_received_total",
		Help: "Total number of blocks received",
	})

	BlockHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "btc_block_height",
		Help: "Latest block height observed",
	})

	BlockTxCount = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "btc_block_transaction_count",
		Help:    "Number of transactions per block",
		Buckets: []float64{100, 500, 1000, 2000, 3000, 4000, 5000, 7500, 10000},
	})

	// Peer metrics
	PeersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "btc_peers_active",
		Help: "Number of currently active peer connections",
	})

	PeersByRegion = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "btc_peers_by_region",
		Help: "Number of active peers by region",
	}, []string{"region"})

	PeerConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_peer_connections_total",
		Help: "Total number of peer connection attempts",
	})

	PeerDisconnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_peer_disconnections_total",
		Help: "Total number of peer disconnections",
	})

	PeerHandshakeFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_peer_handshake_failures_total",
		Help: "Total number of handshake failures",
	})

	PeerLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "btc_peer_latency_ms",
		Help:    "Peer latency in milliseconds",
		Buckets: []float64{10, 25, 50, 100, 200, 500, 1000, 2000, 5000},
	}, []string{"region"})

	// Database metrics
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "btc_db_query_duration_seconds",
		Help:    "Database query duration in seconds",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"operation"})

	DBErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "btc_db_errors_total",
		Help: "Total number of database errors",
	}, []string{"operation"})

	// Inv message metrics
	InvTxAnnouncements = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_inv_tx_announcements_total",
		Help: "Total transaction announcements received via inv messages",
	})

	InvBlockAnnouncements = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_inv_block_announcements_total",
		Help: "Total block announcements received via inv messages",
	})

	// Dedup metrics
	TxDeduplicated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "btc_tx_deduplicated_total",
		Help: "Total transactions skipped due to deduplication",
	})

	SeenMapSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "btc_seen_map_size",
		Help: "Current size of seen maps",
	}, []string{"type"})
)

// SeedFromDB initializes counter metrics from historical database totals
// so they don't reset to zero on restart.
func SeedFromDB(db *sql.DB) {
	var txReceived, txRecorded, conflicts, blocks float64
	var blockHeight sql.NullFloat64
	var invTx, invBlock float64

	row := db.QueryRow(`
		SELECT
			COALESCE((SELECT COUNT(*) FROM transaction_observations), 0),
			COALESCE((SELECT COUNT(*) FROM transactions), 0),
			COALESCE((SELECT COUNT(*) FROM transaction_observations WHERE double_spend_flag = TRUE), 0),
			COALESCE((SELECT COUNT(*) FROM blocks), 0),
			(SELECT MAX(height) FROM blocks),
			COALESCE((SELECT SUM(COALESCE(tx_announcements, 0)) FROM peer_connections), 0),
			COALESCE((SELECT SUM(COALESCE(block_announcements, 0)) FROM peer_connections), 0)
	`)

	if err := row.Scan(&txReceived, &txRecorded, &conflicts, &blocks, &blockHeight, &invTx, &invBlock); err != nil {
		log.Printf("Failed to seed metrics from database: %v", err)
		return
	}

	TxReceived.Add(txReceived)
	TxRecordedDB.Add(txRecorded)
	TxConflicts.Add(conflicts)
	BlocksReceived.Add(blocks)
	InvTxAnnouncements.Add(invTx)
	InvBlockAnnouncements.Add(invBlock)

	if blockHeight.Valid {
		BlockHeight.Set(blockHeight.Float64)
	}

	log.Printf("Seeded metrics from DB: %d tx received, %d recorded, %d blocks, height %.0f",
		int(txReceived), int(txRecorded), int(blocks), blockHeight.Float64)
}

// corsHandler wraps a handler with CORS headers
func corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// StartMetricsServer starts the Prometheus metrics HTTP server
func StartMetricsServer(addr string) {
	http.Handle("/metrics", corsHandler(promhttp.Handler()))
	go http.ListenAndServe(addr, nil)
}
