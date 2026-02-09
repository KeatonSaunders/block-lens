package observer

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/keato/btc-observer/internal/database"
	"github.com/keato/btc-observer/internal/logger"
	"github.com/keato/btc-observer/internal/metrics"
	"github.com/keato/btc-observer/internal/protocol"
	"github.com/rs/zerolog"
)

// activeConns tracks all active connections for graceful shutdown
var activeConns = struct {
	sync.Mutex
	conns map[net.Conn]struct{}
}{conns: make(map[net.Conn]struct{})}

func trackConn(conn net.Conn) {
	activeConns.Lock()
	activeConns.conns[conn] = struct{}{}
	activeConns.Unlock()
}

func untrackConn(conn net.Conn) {
	activeConns.Lock()
	delete(activeConns.conns, conn)
	activeConns.Unlock()
}

// CloseAllConnections closes all active peer connections
func CloseAllConnections() {
	activeConns.Lock()
	defer activeConns.Unlock()
	for conn := range activeConns.conns {
		conn.Close()
	}
}

// ObserveNode connects to a node and processes messages
func ObserveNode(ctx context.Context, node *Node, country string, pm *PeerManager, db *database.DB, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}

	addr := node.Addr()
	plog := logger.PeerLogger(country, addr)

	plog.Info().Str("city", node.City).Str("country", node.CountryCode).Msg("Connecting")
	metrics.PeerConnections.Inc()

	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		plog.Warn().Err(err).Msg("Connection failed")
		pm.MarkFailed(addr)
		return
	}
	defer conn.Close()

	trackConn(conn)
	defer untrackConn(conn)

	// Perform handshake
	if err := doHandshake(conn, addr, plog, db); err != nil {
		plog.Warn().Err(err).Msg("Handshake failed")
		metrics.PeerHandshakeFailures.Inc()
		pm.MarkFailed(addr)
		return
	}

	// Update geo info in database
	geoInfo := &database.PeerGeoInfo{
		CountryCode: node.CountryCode,
		City:        node.City,
		Region:      country, // Use country as region for backwards compatibility
		Latitude:    node.Latitude,
		Longitude:   node.Longitude,
		ASN:         node.ASN,
		OrgName:     node.OrgName,
	}
	if err := db.UpdatePeerGeoInfo(addr, geoInfo); err != nil {
		plog.Error().Err(err).Msg("DB UpdatePeerGeoInfo error")
	}

	pm.SetActive(country, addr, node)
	connectedAt := time.Now()
	metrics.PeersActive.Inc()
	metrics.PeersByRegion.WithLabelValues(country).Inc()
	plog.Info().Str("city", node.City).Str("country", node.CountryCode).Msg("Connected")

	// Run message loop
	runMessageLoop(ctx, conn, addr, country, plog, db)

	pm.RemoveActive(country, addr)
	metrics.PeersActive.Dec()
	metrics.PeersByRegion.WithLabelValues(country).Dec()
	metrics.PeerDisconnections.Inc()

	// Track disconnection - if connection lasted less than 1 minute, it's suspicious
	if time.Since(connectedAt) < time.Minute {
		pm.MarkDisconnect(addr)
		plog.Warn().Msg("Disconnected (short-lived)")
	} else {
		plog.Info().Msg("Disconnected")
	}
}

func doHandshake(conn net.Conn, address string, plog zerolog.Logger, db *database.DB) error {
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// Create and send version message
	versionMsg := protocol.CreateVersionMessage(conn.RemoteAddr().String())
	versionBytes, err := protocol.EncodeVersionMessage(versionMsg)
	if err != nil {
		return fmt.Errorf("encode version: %w", err)
	}

	versionPacket := protocol.CreateMessagePacket("version", versionBytes)
	if _, err := conn.Write(versionPacket); err != nil {
		return fmt.Errorf("send version: %w", err)
	}

	// Receive peer's version message
	peerVersion, err := protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	// Parse and record peer version info
	peerVersionData, err := protocol.ParseVersionMessage(peerVersion.Payload)
	if err != nil {
		return fmt.Errorf("parse version: %w", err)
	}

	if err := db.RecordPeerConnection(address, peerVersionData); err != nil {
		plog.Error().Err(err).Msg("DB RecordPeerConnection error")
	}

	// Send verack
	verackPacket := protocol.CreateMessagePacket("verack", []byte{})
	if _, err := conn.Write(verackPacket); err != nil {
		return fmt.Errorf("send verack: %w", err)
	}

	// Receive peer's verack
	_, err = protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("read verack: %w", err)
	}

	return nil
}

func runMessageLoop(ctx context.Context, conn net.Conn, address, region string, plog zerolog.Logger, db *database.DB) {
	peerAddr := conn.RemoteAddr().String()
	var pendingPingTime time.Time

	txCount := 0
	blockCount := 0
	lastSummary := time.Now()

	for {
		// Check for shutdown signal
		select {
		case <-ctx.Done():
			plog.Info().Msg("Shutting down")
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(10 * time.Minute))

		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			if ctx.Err() != nil {
				plog.Info().Msg("Shutdown complete")
				return
			}
			if err == io.EOF {
				plog.Info().Msg("Connection closed by peer")
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				plog.Warn().Msg("Connection timeout")
			} else {
				plog.Warn().Err(err).Msg("Read error")
			}
			return
		}

		command := protocol.CommandString(msg)

		switch command {
		case "inv":
			handleInv(conn, msg, address, peerAddr, plog, db)

		case "tx":
			tx, err := protocol.ParseTxMessage(msg.Payload)
			if err != nil {
				continue
			}
			txCount++
			metrics.TxReceived.Inc()
			if err := db.RecordTransaction(tx); err != nil {
				plog.Error().Err(err).Msg("DB RecordTransaction error")
			} else {
				metrics.TxRecordedDB.Inc()
			}
			db.DetectInputConflicts(tx)

		case "block":
			block, err := protocol.ParseBlockMessage(msg.Payload)
			if err != nil {
				continue
			}
			plog.Info().
				Str("hash", fmt.Sprintf("%x", protocol.ReverseBytes(block.BlockHash[:]))).
				Int("height", int(block.Height)).
				Int("txs", len(block.Transactions)).
				Msg("BLOCK")
			blockCount++
			metrics.BlocksReceived.Inc()
			metrics.BlockHeight.Set(float64(block.Height))
			metrics.BlockTxCount.Observe(float64(len(block.Transactions)))

			db.RecordBlock(block, peerAddr)
			for _, tx := range block.Transactions {
				db.RecordTransaction(tx)
			}

			txHashes := make([][]byte, len(block.Transactions))
			for i, tx := range block.Transactions {
				txHashes[i] = tx.TxID[:]
			}
			blockTime := time.Unix(int64(block.Header.Timestamp), 0)
			db.ConfirmTransactions(block.BlockHash[:], int(block.Height), blockTime, txHashes)

		case "ping":
			pongPacket := protocol.CreateMessagePacket("pong", msg.Payload)
			conn.Write(pongPacket)

		case "pong":
			if !pendingPingTime.IsZero() {
				latencyMs := int(time.Since(pendingPingTime).Milliseconds())
				db.UpdatePeerLatency(address, latencyMs)
				metrics.PeerLatency.WithLabelValues(region).Observe(float64(latencyMs))
				pendingPingTime = time.Time{}
			}
		}

		if time.Since(lastSummary) >= 60*time.Second {
			plog.Info().Int("txs", txCount).Int("blocks", blockCount).Msg("Status")
			txCount = 0
			blockCount = 0
			lastSummary = time.Now()

			// Send ping to measure latency
			var nonce [8]byte
			if _, err := rand.Read(nonce[:]); err == nil {
				pingPacket := protocol.CreateMessagePacket("ping", nonce[:])
				if _, err := conn.Write(pingPacket); err == nil {
					pendingPingTime = time.Now()
				}
			}
		}
	}
}

func handleInv(conn net.Conn, msg *protocol.Message, address, peerAddr string, plog zerolog.Logger, db *database.DB) {
	inv := protocol.ParseInvMessage(msg.Payload)

	// Record observations
	for _, v := range inv.TxVectors {
		if err := db.RecordObservation(v.Hash[:], peerAddr); err != nil {
			plog.Error().Err(err).Msg("DB RecordObservation error")
		}
	}

	// Update announcement counts and metrics
	if inv.TxCount > 0 {
		metrics.InvTxAnnouncements.Add(float64(inv.TxCount))
	}
	if inv.BlockCount > 0 {
		metrics.InvBlockAnnouncements.Add(float64(inv.BlockCount))
	}
	if inv.TxCount > 0 || inv.BlockCount > 0 {
		if err := db.IncrementPeerAnnouncements(address, inv.TxCount, inv.BlockCount); err != nil {
			plog.Error().Err(err).Msg("DB IncrementPeerAnnouncements error")
		}
	}

	// Request new transactions
	var newTxVectors []protocol.InvVector
	for _, v := range inv.TxVectors {
		if MarkSeenTx(v.Hash) {
			newTxVectors = append(newTxVectors, v)
		} else {
			metrics.TxDeduplicated.Inc()
		}
	}
	if len(newTxVectors) > 0 {
		getDataPayload := protocol.CreateGetDataPayload(newTxVectors)
		getDataPacket := protocol.CreateMessagePacket("getdata", getDataPayload)
		conn.Write(getDataPacket)
	}

	// Request new blocks
	var newBlockVectors []protocol.InvVector
	for _, v := range inv.BlockVectors {
		if MarkSeenBlock(v.Hash) {
			newBlockVectors = append(newBlockVectors, v)
		}
	}
	if len(newBlockVectors) > 0 {
		getDataPayload := protocol.CreateGetDataPayload(newBlockVectors)
		getDataPacket := protocol.CreateMessagePacket("getdata", getDataPayload)
		conn.Write(getDataPacket)
	}
}

// StartPeerManager starts the peer manager loop that maintains connections
func StartPeerManager(ctx context.Context, pm *PeerManager, db *database.DB, wg *sync.WaitGroup) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			for _, country := range TargetCountries {
				active := pm.ActiveCountByCountry(country)
				if active < PeersPerCountry {
					if node, ok := pm.GetNextPeer(country); ok {
						wg.Add(1)
						go ObserveNode(ctx, node, country, pm, db, wg)
					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

// StartStatusReporter starts periodic status logging
func StartStatusReporter(ctx context.Context, pm *PeerManager, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Log.Info().
					Int("total", pm.TotalActive()).
					Str("regions", pm.Status()).
					Msg("Peer status")
			}
		}
	}()
}
