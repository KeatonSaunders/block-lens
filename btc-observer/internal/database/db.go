package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/keato/btc-observer/internal/protocol"
	_ "github.com/lib/pq"
)

type DB struct {
	conn *sql.DB
}

type Config struct {
	DBHost     string `json:"db_host"`
	DBPort     int    `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBName     string `json:"db_name"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Environment variables override config file values
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPassword = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := fmt.Sscanf(v, "%d", &cfg.DBPort); port != 1 || err != nil {
			return nil, fmt.Errorf("invalid DB_PORT: %s", v)
		}
	}

	return &cfg, nil
}

func New(host string, port int, user, password, dbname string) (*DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

func NewFromConfig(cfg *Config) (*DB, error) {
	return New(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
}

func (db *DB) Close() error {
	return db.conn.Close()
}

// PeerGeoInfo holds geolocation data for a peer
type PeerGeoInfo struct {
	CountryCode string
	City        string
	Region      string
	Latitude    float64
	Longitude   float64
	ASN         string
	OrgName     string
}

func (db *DB) RecordPeerConnection(peerAddr string, version *protocol.VersionMessage) error {
	_, err := db.conn.Exec(
		`INSERT INTO peer_connections (peer_addr, first_connected_at, last_seen_at, protocol_version, user_agent, services, connection_count)
		 VALUES ($1, NOW(), NOW(), $2, $3, $4, 1)
		 ON CONFLICT (peer_addr) DO UPDATE SET
		     last_seen_at = NOW(),
		     protocol_version = $2,
		     user_agent = $3,
		     services = $4,
		     connection_count = peer_connections.connection_count + 1`,
		peerAddr, version.Version, version.UserAgent, version.Services,
	)
	return err
}

func (db *DB) UpdatePeerGeoInfo(peerAddr string, geo *PeerGeoInfo) error {
	_, err := db.conn.Exec(
		`UPDATE peer_connections SET
		     country_code = $2,
		     city = $3,
		     region = $4,
		     latitude = $5,
		     longitude = $6,
		     asn = $7,
		     org_name = $8
		 WHERE peer_addr = $1`,
		peerAddr, geo.CountryCode, geo.City, geo.Region,
		geo.Latitude, geo.Longitude, geo.ASN, geo.OrgName,
	)
	return err
}

func (db *DB) IncrementPeerAnnouncements(peerAddr string, txCount, blockCount int) error {
	_, err := db.conn.Exec(
		`UPDATE peer_connections SET
		     tx_announcements = COALESCE(tx_announcements, 0) + $2,
		     block_announcements = COALESCE(block_announcements, 0) + $3,
		     last_seen_at = NOW()
		 WHERE peer_addr = $1`,
		peerAddr, txCount, blockCount,
	)
	return err
}

func (db *DB) UpdatePeerLatency(peerAddr string, latencyMs int) error {
	_, err := db.conn.Exec(
		`UPDATE peer_connections SET
		     avg_latency_ms = CASE
		         WHEN avg_latency_ms IS NULL THEN $2
		         ELSE (avg_latency_ms + $2) / 2
		     END,
		     last_seen_at = NOW()
		 WHERE peer_addr = $1`,
		peerAddr, latencyMs,
	)
	return err
}


func (db *DB) RecordObservation(txHash []byte, peerAddr string) error {
	_, err := db.conn.Exec(
		`INSERT INTO transaction_observations (tx_hash, first_seen_at, first_peer_addr)
		 VALUES ($1, NOW(), $2)
		 ON CONFLICT (tx_hash) DO UPDATE SET peer_count = transaction_observations.peer_count + 1`,
		txHash, peerAddr,
	)
	if err != nil {
		return err
	}

	// Record propagation event with delay from first observation
	_, err = db.conn.Exec(
		`INSERT INTO propagation_events (tx_hash, peer_addr, announcement_time, delay_from_first_ms)
		 VALUES ($1, $2, NOW(),
		     COALESCE(
		         EXTRACT(EPOCH FROM (NOW() - (SELECT first_seen_at FROM transaction_observations WHERE tx_hash = $1))) * 1000,
		         0
		     )::INT
		 )`,
		txHash, peerAddr,
	)
	return err
}

func (db *DB) RecordTransaction(tx *protocol.Transaction) error {
	dbTx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer dbTx.Rollback()

	totalOutput := int64(0)
	for _, out := range tx.Outputs {
		totalOutput += out.Value
	}

	// Calculate weight: non-witness data * 4 + witness data
	// For non-segwit: weight = size * 4
	// For segwit: we'd need to track witness size separately (approximation for now)
	weight := tx.SizeBytes * 4
	if tx.Segwit {
		// Rough approximation: segwit txs are ~25% witness data on average
		weight = tx.SizeBytes * 3
	}

	_, err = dbTx.Exec(
		`INSERT INTO transactions (tx_hash, size_bytes, weight, input_count, output_count, total_output)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tx.TxID[:], tx.SizeBytes, weight, len(tx.Inputs), len(tx.Outputs), totalOutput,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	totalInput := int64(0)
	inputsFound := 0
	for i, in := range tx.Inputs {
		// Look up address and value from the output being spent
		var address sql.NullString
		var valueSatoshis sql.NullInt64
		dbTx.QueryRow(
			`SELECT address, value_satoshis FROM transaction_outputs
			 WHERE tx_hash = $1 AND output_index = $2`,
			in.PrevTxHash[:], in.PrevIndex,
		).Scan(&address, &valueSatoshis)

		if valueSatoshis.Valid {
			totalInput += valueSatoshis.Int64
			inputsFound++
		}

		_, err = dbTx.Exec(
			`INSERT INTO transaction_inputs (tx_hash, input_index, prev_tx_hash, prev_output_idx, script_sig, address, value_satoshis)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT DO NOTHING`,
			tx.TxID[:], i, in.PrevTxHash[:], in.PrevIndex, in.ScriptSig,
			address, valueSatoshis,
		)
		if err != nil {
			return fmt.Errorf("insert input %d: %w", i, err)
		}

		// Mark the spent output
		_, err = dbTx.Exec(
			`UPDATE transaction_outputs
			 SET spent_in_tx = $1, spent_at = NOW()
			 WHERE tx_hash = $2 AND output_index = $3 AND spent_in_tx IS NULL`,
			tx.TxID[:], in.PrevTxHash[:], in.PrevIndex,
		)
		if err != nil {
			return fmt.Errorf("mark output spent %d: %w", i, err)
		}
	}

	// Update total_input and fee only if we found ALL input values
	if inputsFound == len(tx.Inputs) && totalInput > 0 {
		fee := totalInput - totalOutput
		_, err = dbTx.Exec(
			`UPDATE transactions SET total_input = $2, fee_satoshis = $3 WHERE tx_hash = $1`,
			tx.TxID[:], totalInput, fee,
		)
		if err != nil {
			return fmt.Errorf("update fee: %w", err)
		}
	}

	for i, out := range tx.Outputs {
		addr := protocol.ExtractAddress(out.ScriptPubKey)
		_, err = dbTx.Exec(
			`INSERT INTO transaction_outputs (tx_hash, output_index, value_satoshis, script_pubkey, address)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT DO NOTHING`,
			tx.TxID[:], i, out.Value, out.ScriptPubKey,
			sql.NullString{String: addr, Valid: addr != ""},
		)
		if err != nil {
			return fmt.Errorf("insert output %d: %w", i, err)
		}
	}

	return dbTx.Commit()
}

func (db *DB) RecordBlock(block *protocol.Block, peerAddr string) error {
	_, err := db.conn.Exec(
		`INSERT INTO blocks (block_hash, height, prev_block_hash, merkle_root, timestamp, difficulty, nonce, tx_count, first_seen_at, first_peer_addr)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)
		 ON CONFLICT DO NOTHING`,
		block.BlockHash[:],
		block.Height,
		block.Header.PrevBlockHash[:],
		block.Header.MerkleRoot[:],
		time.Unix(int64(block.Header.Timestamp), 0),
		block.Difficulty,
		int64(block.Header.Nonce),
		len(block.Transactions),
		peerAddr,
	)
	return err
}

func (db *DB) DetectInputConflicts(tx *protocol.Transaction) error {
	var zeroHash [32]byte

	// Collect conflicting tx hashes across all inputs
	var conflictingTxHashes [][]byte
	for _, in := range tx.Inputs {
		// Skip coinbase inputs
		if bytes.Equal(in.PrevTxHash[:], zeroHash[:]) {
			continue
		}

		rows, err := db.conn.Query(
			`SELECT DISTINCT ti.tx_hash
			 FROM transaction_inputs ti
			 JOIN transactions t ON ti.tx_hash = t.tx_hash
			 WHERE ti.prev_tx_hash = $1 AND ti.prev_output_idx = $2
			   AND t.block_hash IS NULL
			   AND ti.tx_hash != $3`,
			in.PrevTxHash[:], in.PrevIndex, tx.TxID[:],
		)
		if err != nil {
			return fmt.Errorf("query conflicts: %w", err)
		}

		for rows.Next() {
			var txHash []byte
			if err := rows.Scan(&txHash); err != nil {
				rows.Close()
				return fmt.Errorf("scan conflict: %w", err)
			}
			conflictingTxHashes = append(conflictingTxHashes, txHash)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows error: %w", err)
		}
	}

	if len(conflictingTxHashes) == 0 {
		return nil
	}

	// Flag all conflicts in a single DB transaction
	dbTx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer dbTx.Rollback()

	for _, oldTxHash := range conflictingTxHashes {
		_ = oldTxHash

		// Flag the old transaction's observation
		_, err := dbTx.Exec(
			`UPDATE transaction_observations
			 SET replaced_by_tx = $1, double_spend_flag = TRUE
			 WHERE tx_hash = $2 AND replaced_by_tx IS NULL`,
			tx.TxID[:], oldTxHash,
		)
		if err != nil {
			return fmt.Errorf("flag old tx: %w", err)
		}
	}

	// Flag the new transaction's observation
	_, err = dbTx.Exec(
		`UPDATE transaction_observations
		 SET double_spend_flag = TRUE
		 WHERE tx_hash = $1`,
		tx.TxID[:],
	)
	if err != nil {
		return fmt.Errorf("flag new tx: %w", err)
	}

	return dbTx.Commit()
}

func (db *DB) ConfirmTransactions(blockHash []byte, blockHeight int, blockTimestamp time.Time, txHashes [][]byte) error {
	dbTx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer dbTx.Rollback()

	for _, txHash := range txHashes {
		_, err = dbTx.Exec(
			`UPDATE transactions SET block_hash = $1, block_height = $2
			 WHERE tx_hash = $3 AND block_hash IS NULL`,
			blockHash, blockHeight, txHash,
		)
		if err != nil {
			return fmt.Errorf("update transaction: %w", err)
		}

		_, err = dbTx.Exec(
			`UPDATE transaction_observations
			 SET in_block_hash = $1, confirmed_at = $2
			 WHERE tx_hash = $3 AND in_block_hash IS NULL`,
			blockHash, blockTimestamp, txHash,
		)
		if err != nil {
			return fmt.Errorf("update observation: %w", err)
		}
	}

	return dbTx.Commit()
}
