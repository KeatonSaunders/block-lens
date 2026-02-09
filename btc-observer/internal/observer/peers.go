package observer

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/keato/btc-observer/internal/logger"
)

const (
	PeersPerCountry  = 1
	failBackoff      = 5 * time.Minute
	disconnectWindow = 2 * time.Minute
	maxStrikes       = 2
)

// TargetCountries defines the countries we want to connect to
var TargetCountries = []string{
	// South America
	"BR", "AR",
	// Africa
	"ZA", "NG", "KE",
	// North America
	"US", "CA",
	// Europe
	"DE", "NL", "RU",
	// Asia
	"JP", "SG", "IN", "AE", "MY", "TH",
	// Oceania
	"AU", "NZ",
}

// targetCountrySet for O(1) lookup
var targetCountrySet = func() map[string]bool {
	m := make(map[string]bool)
	for _, c := range TargetCountries {
		m[c] = true
	}
	return m
}()

// Node represents a Bitcoin node with geolocation info
type Node struct {
	Address     string
	Port        int
	Version     int
	UserAgent   string
	City        string
	CountryCode string
	Latitude    float64
	Longitude   float64
	ASN         string
	OrgName     string
}

// Addr returns the address:port string
func (n *Node) Addr() string {
	return fmt.Sprintf("%s:%d", n.Address, n.Port)
}

// PeerManager tracks active peers by country
type PeerManager struct {
	sync.RWMutex
	activeByCountry map[string]map[string]*Node // country -> addr -> node
	available       map[string][]*Node          // country -> nodes
	failed          map[string]time.Time
	strikes         map[string]int
	lastDisconnect  map[string]time.Time
	blacklist       map[string]bool
}

// NewPeerManager creates a new peer manager
func NewPeerManager() *PeerManager {
	return &PeerManager{
		activeByCountry: make(map[string]map[string]*Node),
		available:       make(map[string][]*Node),
		failed:          make(map[string]time.Time),
		strikes:         make(map[string]int),
		lastDisconnect:  make(map[string]time.Time),
		blacklist:       make(map[string]bool),
	}
}

// SetActive marks a peer as actively connected
func (pm *PeerManager) SetActive(country, addr string, node *Node) {
	pm.Lock()
	defer pm.Unlock()
	if pm.activeByCountry[country] == nil {
		pm.activeByCountry[country] = make(map[string]*Node)
	}
	pm.activeByCountry[country][addr] = node
}

// RemoveActive removes a peer from active connections
func (pm *PeerManager) RemoveActive(country, addr string) {
	pm.Lock()
	defer pm.Unlock()
	if pm.activeByCountry[country] != nil {
		delete(pm.activeByCountry[country], addr)
	}
}

// ActiveCountByCountry returns the number of active peers in a country
func (pm *PeerManager) ActiveCountByCountry(country string) int {
	pm.RLock()
	defer pm.RUnlock()
	return len(pm.activeByCountry[country])
}

// TotalActive returns the total number of active peers
func (pm *PeerManager) TotalActive() int {
	pm.RLock()
	defer pm.RUnlock()
	total := 0
	for _, countryPeers := range pm.activeByCountry {
		total += len(countryPeers)
	}
	return total
}

// SetAvailable sets the available nodes for a country
func (pm *PeerManager) SetAvailable(country string, nodes []*Node) {
	pm.Lock()
	defer pm.Unlock()
	pm.available[country] = nodes
}

// GetNextPeer returns the next available peer for a country
func (pm *PeerManager) GetNextPeer(country string) (*Node, bool) {
	pm.Lock()
	defer pm.Unlock()

	nodes := pm.available[country]
	active := pm.activeByCountry[country]
	if active == nil {
		active = make(map[string]*Node)
	}

	now := time.Now()
	for _, node := range nodes {
		addr := node.Addr()
		if pm.blacklist[addr] {
			continue
		}
		if _, isActive := active[addr]; isActive {
			continue
		}
		if lastFail, failed := pm.failed[addr]; failed && now.Sub(lastFail) < failBackoff {
			continue
		}
		return node, true
	}
	return nil, false
}

// MarkFailed marks a peer as failed (connection or handshake failure)
func (pm *PeerManager) MarkFailed(addr string) {
	pm.Lock()
	defer pm.Unlock()
	pm.failed[addr] = time.Now()
}

// MarkDisconnect tracks rapid disconnections and blacklists problematic peers
func (pm *PeerManager) MarkDisconnect(addr string) {
	pm.Lock()
	defer pm.Unlock()

	now := time.Now()
	if lastDc, ok := pm.lastDisconnect[addr]; ok && now.Sub(lastDc) < disconnectWindow {
		pm.strikes[addr]++
		if pm.strikes[addr] >= maxStrikes {
			pm.blacklist[addr] = true
			logger.Log.Warn().Str("peer", addr).Msg("Blacklisted peer (repeated rapid disconnections)")
		}
	} else {
		pm.strikes[addr] = 1
	}
	pm.lastDisconnect[addr] = now
	pm.failed[addr] = now
}

// Status returns a string summarizing active peers by country
func (pm *PeerManager) Status() string {
	pm.RLock()
	defer pm.RUnlock()

	// Sort countries for consistent output
	countries := make([]string, 0, len(pm.activeByCountry))
	for country := range pm.activeByCountry {
		if len(pm.activeByCountry[country]) > 0 {
			countries = append(countries, country)
		}
	}
	sort.Strings(countries)

	parts := make([]string, 0, len(countries))
	for _, country := range countries {
		parts = append(parts, country)
	}
	return strings.Join(parts, ",")
}

// IsTargetCountry checks if a country code is in our target list
func IsTargetCountry(countryCode string) bool {
	return targetCountrySet[countryCode]
}
