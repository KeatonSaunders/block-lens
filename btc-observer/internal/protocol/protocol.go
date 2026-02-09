package protocol

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// Bitcoin Protocol Constants
const (
	MagicMainnet       = 0xD9B4BEF9
	ProtocolVersion    = 70015
	ServicesNone       = 0
	ServicesNodeNetwork = 1
)

// Message represents a Bitcoin protocol message
type Message struct {
	Magic    uint32
	Command  [12]byte
	Length   uint32
	Checksum [4]byte
	Payload  []byte
}

// NetworkAddress represents a Bitcoin network address
type NetworkAddress struct {
	Services uint64
	IP       [16]byte
	Port     uint16
}

// VersionMessage is the first message sent in the handshake
type VersionMessage struct {
	Version     int32
	Services    uint64
	Timestamp   int64
	AddrRecv    NetworkAddress
	AddrFrom    NetworkAddress
	Nonce       uint64
	UserAgent   string
	StartHeight int32
	Relay       bool
}

// InvVector is a single inventory item (type + hash)
type InvVector struct {
	Type uint32
	Hash [32]byte
}

// InvResult holds parsed inventory message results
type InvResult struct {
	TxCount      int
	BlockCount   int
	TxVectors    []InvVector
	BlockVectors []InvVector
}

// TxInput represents a parsed transaction input
type TxInput struct {
	PrevTxHash [32]byte
	PrevIndex  uint32
	ScriptSig  []byte
	Sequence   uint32
}

// TxOutput represents a parsed transaction output
type TxOutput struct {
	Value        int64
	ScriptPubKey []byte
}

// Transaction holds a fully parsed Bitcoin transaction
type Transaction struct {
	Version   int32
	Inputs    []TxInput
	Outputs   []TxOutput
	LockTime  uint32
	TxID      [32]byte
	Segwit    bool
	SizeBytes int
}

// BlockHeader represents a parsed Bitcoin block header
type BlockHeader struct {
	Version       int32
	PrevBlockHash [32]byte
	MerkleRoot    [32]byte
	Timestamp     uint32
	Bits          uint32
	Nonce         uint32
}

// Block represents a parsed Bitcoin block
type Block struct {
	Header       BlockHeader
	BlockHash    [32]byte
	Height       int32
	Difficulty   float64
	Transactions []*Transaction
}

// CommandString extracts the command name from a message's null-padded 12-byte field.
func CommandString(msg *Message) string {
	return string(bytes.Trim(msg.Command[:], "\x00"))
}

// CreateMessagePacket wraps payload in Bitcoin message format
func CreateMessagePacket(command string, payload []byte) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, uint32(MagicMainnet))

	cmd := [12]byte{}
	copy(cmd[:], command)
	buf.Write(cmd[:])

	binary.Write(buf, binary.LittleEndian, uint32(len(payload)))

	checksum := calculateChecksum(payload)
	buf.Write(checksum[:])

	buf.Write(payload)

	return buf.Bytes()
}

// ReadMessage reads and parses a Bitcoin protocol message from a connection.
func ReadMessage(conn net.Conn) (*Message, error) {
	msg := &Message{}

	header := make([]byte, 24)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	buf := bytes.NewReader(header)

	binary.Read(buf, binary.LittleEndian, &msg.Magic)
	io.ReadFull(buf, msg.Command[:])
	binary.Read(buf, binary.LittleEndian, &msg.Length)
	io.ReadFull(buf, msg.Checksum[:])

	if msg.Magic != MagicMainnet {
		return nil, fmt.Errorf("invalid magic bytes: 0x%x (expected 0x%x)", msg.Magic, MagicMainnet)
	}

	if msg.Length > 0 {
		msg.Payload = make([]byte, msg.Length)
		if _, err := io.ReadFull(conn, msg.Payload); err != nil {
			return nil, err
		}

		expectedChecksum := calculateChecksum(msg.Payload)
		if !bytes.Equal(msg.Checksum[:], expectedChecksum[:]) {
			return nil, fmt.Errorf("checksum mismatch")
		}
	}

	return msg, nil
}

// CreateVersionMessage builds a version message for the handshake.
func CreateVersionMessage(peerAddr string) *VersionMessage {
	var nonce uint64
	binary.Read(rand.Reader, binary.LittleEndian, &nonce)

	host, portStr, _ := net.SplitHostPort(peerAddr)
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)

	return &VersionMessage{
		Version:     ProtocolVersion,
		Services:    ServicesNone,
		Timestamp:   time.Now().Unix(),
		AddrRecv:    createNetworkAddress(host, port, ServicesNodeNetwork),
		AddrFrom:    createNetworkAddress("0.0.0.0", 0, ServicesNone),
		Nonce:       nonce,
		UserAgent:   "/btc-observer:0.1.0/",
		StartHeight: 0,
		Relay:       true,
	}
}

// EncodeVersionMessage serializes the version message to bytes.
func EncodeVersionMessage(v *VersionMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, v.Version)
	binary.Write(buf, binary.LittleEndian, v.Services)
	binary.Write(buf, binary.LittleEndian, v.Timestamp)

	binary.Write(buf, binary.LittleEndian, v.AddrRecv.Services)
	buf.Write(v.AddrRecv.IP[:])
	binary.Write(buf, binary.BigEndian, v.AddrRecv.Port)

	binary.Write(buf, binary.LittleEndian, v.AddrFrom.Services)
	buf.Write(v.AddrFrom.IP[:])
	binary.Write(buf, binary.BigEndian, v.AddrFrom.Port)

	binary.Write(buf, binary.LittleEndian, v.Nonce)

	writeVarString(buf, v.UserAgent)

	binary.Write(buf, binary.LittleEndian, v.StartHeight)

	if v.Version >= 70001 {
		if v.Relay {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	}

	return buf.Bytes(), nil
}

// ParseVersionMessage parses a version message payload from a peer.
func ParseVersionMessage(payload []byte) (*VersionMessage, error) {
	if len(payload) < 80 {
		return nil, fmt.Errorf("version payload too short: %d bytes", len(payload))
	}

	buf := bytes.NewReader(payload)
	v := &VersionMessage{}

	binary.Read(buf, binary.LittleEndian, &v.Version)
	binary.Read(buf, binary.LittleEndian, &v.Services)
	binary.Read(buf, binary.LittleEndian, &v.Timestamp)

	// AddrRecv
	binary.Read(buf, binary.LittleEndian, &v.AddrRecv.Services)
	io.ReadFull(buf, v.AddrRecv.IP[:])
	binary.Read(buf, binary.BigEndian, &v.AddrRecv.Port)

	// AddrFrom
	binary.Read(buf, binary.LittleEndian, &v.AddrFrom.Services)
	io.ReadFull(buf, v.AddrFrom.IP[:])
	binary.Read(buf, binary.BigEndian, &v.AddrFrom.Port)

	binary.Read(buf, binary.LittleEndian, &v.Nonce)

	// UserAgent is a var_str
	uaLen, err := readVarInt(buf)
	if err != nil {
		return nil, fmt.Errorf("reading user agent length: %w", err)
	}
	if uaLen > 0 {
		uaBytes := make([]byte, uaLen)
		if _, err := io.ReadFull(buf, uaBytes); err != nil {
			return nil, fmt.Errorf("reading user agent: %w", err)
		}
		v.UserAgent = string(uaBytes)
	}

	binary.Read(buf, binary.LittleEndian, &v.StartHeight)

	// Relay is optional (version >= 70001)
	if v.Version >= 70001 && buf.Len() > 0 {
		var relay byte
		binary.Read(buf, binary.LittleEndian, &relay)
		v.Relay = relay != 0
	}

	return v, nil
}

// ParseAddrMessage parses an addr message and returns a list of peer addresses
func ParseAddrMessage(payload []byte) []string {
	var addrs []string
	buf := bytes.NewReader(payload)

	count, err := readVarInt(buf)
	if err != nil {
		return addrs
	}

	// Limit to prevent memory issues with malformed messages
	if count > 1000 {
		count = 1000
	}

	for i := uint64(0); i < count; i++ {
		// timestamp (4 bytes) - only in addr, not in version
		var timestamp uint32
		if err := binary.Read(buf, binary.LittleEndian, &timestamp); err != nil {
			break
		}

		// services (8 bytes)
		var services uint64
		if err := binary.Read(buf, binary.LittleEndian, &services); err != nil {
			break
		}

		// IP address (16 bytes, IPv6 or IPv4-mapped)
		var ip [16]byte
		if _, err := io.ReadFull(buf, ip[:]); err != nil {
			break
		}

		// port (2 bytes, big endian)
		var port uint16
		if err := binary.Read(buf, binary.BigEndian, &port); err != nil {
			break
		}

		// Convert to address string
		// Check if it's an IPv4-mapped IPv6 address (::ffff:x.x.x.x)
		if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 &&
			ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
			ip[8] == 0 && ip[9] == 0 && ip[10] == 0xff && ip[11] == 0xff {
			// IPv4
			addr := fmt.Sprintf("%d.%d.%d.%d:%d", ip[12], ip[13], ip[14], ip[15], port)
			addrs = append(addrs, addr)
		}
		// Skip IPv6 for now
	}

	return addrs
}

// ParseInvMessage parses an inventory message and returns structured results.
func ParseInvMessage(payload []byte) InvResult {
	result := InvResult{}
	buf := bytes.NewReader(payload)

	count, err := readVarInt(buf)
	if err != nil {
		return result
	}

	for i := uint64(0); i < count; i++ {
		var invType uint32
		var hash [32]byte

		if err := binary.Read(buf, binary.LittleEndian, &invType); err != nil {
			break
		}
		if _, err := io.ReadFull(buf, hash[:]); err != nil {
			break
		}

		switch invType {
		case 1: // MSG_TX
			result.TxCount++
			result.TxVectors = append(result.TxVectors, InvVector{Type: invType, Hash: hash})
		case 2: // MSG_BLOCK
			result.BlockCount++
			result.BlockVectors = append(result.BlockVectors, InvVector{Type: invType, Hash: hash})
		}
	}

	return result
}

// ParseTxMessage parses a raw Bitcoin transaction from a tx message payload.
func ParseTxMessage(payload []byte) (*Transaction, error) {
	buf := bytes.NewReader(payload)
	return parseTxFromReader(buf)
}

// parseTxFromReader parses a single transaction from a reader.
// Used by both ParseTxMessage and ParseBlockMessage.
func parseTxFromReader(buf *bytes.Reader) (*Transaction, error) {
	startLen := buf.Len()

	var version int32
	if err := binary.Read(buf, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("parsing tx version: %w", err)
	}

	segwit := false
	marker, err := buf.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading tx: %w", err)
	}
	if marker == 0x00 {
		flag, err := buf.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading segwit flag: %w", err)
		}
		if flag == 0x01 {
			segwit = true
		}
	} else {
		buf.Seek(-1, io.SeekCurrent)
	}

	inputCount, err := readVarInt(buf)
	if err != nil {
		return nil, fmt.Errorf("reading input count: %w", err)
	}

	inputs := make([]TxInput, inputCount)
	for i := uint64(0); i < inputCount; i++ {
		var prevHash [32]byte
		if _, err := io.ReadFull(buf, prevHash[:]); err != nil {
			return nil, fmt.Errorf("reading input %d: %w", i, err)
		}
		var prevIndex uint32
		binary.Read(buf, binary.LittleEndian, &prevIndex)

		scriptLen, _ := readVarInt(buf)
		scriptSig := make([]byte, scriptLen)
		io.ReadFull(buf, scriptSig)

		var sequence uint32
		binary.Read(buf, binary.LittleEndian, &sequence)

		inputs[i] = TxInput{
			PrevTxHash: prevHash,
			PrevIndex:  prevIndex,
			ScriptSig:  scriptSig,
			Sequence:   sequence,
		}
	}

	outputCount, err := readVarInt(buf)
	if err != nil {
		return nil, fmt.Errorf("reading output count: %w", err)
	}

	outputs := make([]TxOutput, outputCount)
	for i := uint64(0); i < outputCount; i++ {
		var value int64
		binary.Read(buf, binary.LittleEndian, &value)

		scriptLen, _ := readVarInt(buf)
		scriptPubKey := make([]byte, scriptLen)
		io.ReadFull(buf, scriptPubKey)

		outputs[i] = TxOutput{
			Value:        value,
			ScriptPubKey: scriptPubKey,
		}
	}

	if segwit {
		for i := uint64(0); i < inputCount; i++ {
			witnessCount, _ := readVarInt(buf)
			for j := uint64(0); j < witnessCount; j++ {
				itemLen, _ := readVarInt(buf)
				witness := make([]byte, itemLen)
				io.ReadFull(buf, witness)
			}
		}
	}

	var lockTime uint32
	binary.Read(buf, binary.LittleEndian, &lockTime)

	txID := computeTxID(version, inputs, outputs, lockTime)

	return &Transaction{
		Version:   version,
		Inputs:    inputs,
		Outputs:   outputs,
		LockTime:  lockTime,
		TxID:      txID,
		Segwit:    segwit,
		SizeBytes: startLen - buf.Len(),
	}, nil
}

// ParseBlockMessage parses a raw Bitcoin block message payload.
func ParseBlockMessage(payload []byte) (*Block, error) {
	if len(payload) < 80 {
		return nil, fmt.Errorf("block payload too short: %d bytes", len(payload))
	}

	// Compute block hash from the 80-byte header
	headerBytes := payload[:80]
	hash1 := sha256.Sum256(headerBytes)
	hash2 := sha256.Sum256(hash1[:])

	buf := bytes.NewReader(payload)

	var header BlockHeader
	binary.Read(buf, binary.LittleEndian, &header.Version)
	io.ReadFull(buf, header.PrevBlockHash[:])
	io.ReadFull(buf, header.MerkleRoot[:])
	binary.Read(buf, binary.LittleEndian, &header.Timestamp)
	binary.Read(buf, binary.LittleEndian, &header.Bits)
	binary.Read(buf, binary.LittleEndian, &header.Nonce)

	txCount, err := readVarInt(buf)
	if err != nil {
		return nil, fmt.Errorf("reading tx count: %w", err)
	}

	txs := make([]*Transaction, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx, err := parseTxFromReader(buf)
		if err != nil {
			return nil, fmt.Errorf("parsing tx %d in block: %w", i, err)
		}
		txs[i] = tx
	}

	block := &Block{
		Header:       header,
		BlockHash:    hash2,
		Difficulty:   computeDifficulty(header.Bits),
		Transactions: txs,
	}

	// Extract height from coinbase transaction (BIP34)
	if len(txs) > 0 {
		block.Height = extractBlockHeight(txs[0])
	}

	return block, nil
}

// extractBlockHeight reads the block height from the coinbase tx scriptSig (BIP34).
func extractBlockHeight(coinbase *Transaction) int32 {
	script := coinbase.Inputs[0].ScriptSig
	if len(script) < 1 {
		return 0
	}
	numBytes := int(script[0])
	if numBytes == 0 || len(script) < 1+numBytes {
		return 0
	}
	height := int32(0)
	for i := 0; i < numBytes; i++ {
		height |= int32(script[1+i]) << (8 * i)
	}
	return height
}

// CreateGetDataPayload builds a getdata message payload from inv vectors.
func CreateGetDataPayload(vectors []InvVector) []byte {
	buf := new(bytes.Buffer)
	writeVarInt(buf, uint64(len(vectors)))
	for _, v := range vectors {
		binary.Write(buf, binary.LittleEndian, v.Type)
		buf.Write(v.Hash[:])
	}
	return buf.Bytes()
}

// CountAddresses counts addresses in an addr message.
func CountAddresses(payload []byte) int {
	buf := bytes.NewReader(payload)
	count, err := readVarInt(buf)
	if err != nil {
		return 0
	}
	return int(count)
}

// ReverseBytes reverses a byte slice (Bitcoin displays hashes backwards).
func ReverseBytes(b []byte) []byte {
	reversed := make([]byte, len(b))
	for i := 0; i < len(b); i++ {
		reversed[i] = b[len(b)-1-i]
	}
	return reversed
}

// ExtractAddress decodes a scriptPubKey into a Bitcoin address string.
// Returns "" for non-standard or unparseable scripts.
func ExtractAddress(scriptPubKey []byte) string {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(scriptPubKey, &chaincfg.MainNetParams)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0].EncodeAddress()
}

// --- unexported helpers ---

// computeDifficulty converts the compact "bits" field to difficulty.
// difficulty = (0xFFFF * 2^208) / target, where target is decoded from bits.
func computeDifficulty(bits uint32) float64 {
	exponent := bits >> 24
	coefficient := float64(bits & 0x007fffff)
	if coefficient == 0 {
		return 0
	}
	shift := 8 * (int(0x1d) - int(exponent))
	return (0xFFFF / coefficient) * math.Pow(2, float64(shift))
}

func calculateChecksum(data []byte) [4]byte {
	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])
	var checksum [4]byte
	copy(checksum[:], hash2[0:4])
	return checksum
}

func computeTxID(version int32, inputs []TxInput, outputs []TxOutput, lockTime uint32) [32]byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, version)

	writeVarInt(buf, uint64(len(inputs)))
	for _, in := range inputs {
		buf.Write(in.PrevTxHash[:])
		binary.Write(buf, binary.LittleEndian, in.PrevIndex)
		writeVarInt(buf, uint64(len(in.ScriptSig)))
		buf.Write(in.ScriptSig)
		binary.Write(buf, binary.LittleEndian, in.Sequence)
	}

	writeVarInt(buf, uint64(len(outputs)))
	for _, out := range outputs {
		binary.Write(buf, binary.LittleEndian, out.Value)
		writeVarInt(buf, uint64(len(out.ScriptPubKey)))
		buf.Write(out.ScriptPubKey)
	}

	binary.Write(buf, binary.LittleEndian, lockTime)

	hash1 := sha256.Sum256(buf.Bytes())
	hash2 := sha256.Sum256(hash1[:])
	return hash2
}

func writeVarInt(buf *bytes.Buffer, value uint64) {
	if value < 0xfd {
		buf.WriteByte(byte(value))
	} else if value <= 0xffff {
		buf.WriteByte(0xfd)
		binary.Write(buf, binary.LittleEndian, uint16(value))
	} else if value <= 0xffffffff {
		buf.WriteByte(0xfe)
		binary.Write(buf, binary.LittleEndian, uint32(value))
	} else {
		buf.WriteByte(0xff)
		binary.Write(buf, binary.LittleEndian, value)
	}
}

func readVarInt(r io.Reader) (uint64, error) {
	var first byte
	if err := binary.Read(r, binary.LittleEndian, &first); err != nil {
		return 0, err
	}

	switch first {
	case 0xff:
		var value uint64
		err := binary.Read(r, binary.LittleEndian, &value)
		return value, err
	case 0xfe:
		var value uint32
		err := binary.Read(r, binary.LittleEndian, &value)
		return uint64(value), err
	case 0xfd:
		var value uint16
		err := binary.Read(r, binary.LittleEndian, &value)
		return uint64(value), err
	default:
		return uint64(first), nil
	}
}

func writeVarString(buf *bytes.Buffer, s string) {
	writeVarInt(buf, uint64(len(s)))
	buf.WriteString(s)
}

func createNetworkAddress(ip string, port uint16, services uint64) NetworkAddress {
	addr := NetworkAddress{
		Services: services,
		Port:     port,
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		parsedIP = net.ParseIP("0.0.0.0")
	}

	if ipv4 := parsedIP.To4(); ipv4 != nil {
		copy(addr.IP[0:10], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		copy(addr.IP[10:12], []byte{0xff, 0xff})
		copy(addr.IP[12:16], ipv4)
	} else {
		copy(addr.IP[:], parsedIP.To16())
	}

	return addr
}
