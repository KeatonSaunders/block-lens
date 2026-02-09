package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/keato/btc-observer/internal/logger"
)

const (
	bitnodesAPI   = "https://bitnodes.io/api/v1/snapshots/latest/"
	ipGeoBatchAPI = "http://ip-api.com/batch?fields=status,query,country,countryCode,city,lat,lon,isp,org,as"
)

// geoResult holds IP geolocation response
type geoResult struct {
	Status      string  `json:"status"`
	Query       string  `json:"query"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	AS          string  `json:"as"`
}

// lookupGeoBatch fetches geolocation for up to 100 IPs at once
func lookupGeoBatch(ips []string) (map[string]*geoResult, error) {
	body, _ := json.Marshal(ips)
	resp, err := http.Post(ipGeoBatchAPI, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results []geoResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	geoMap := make(map[string]*geoResult)
	for i := range results {
		if results[i].Status == "success" {
			geoMap[results[i].Query] = &results[i]
		}
	}
	return geoMap, nil
}

// FetchNodes retrieves nodes from bitnodes.io and looks up their geolocation
func FetchNodes() (map[string][]*Node, error) {
	logger.Log.Info().Msg("Fetching nodes from bitnodes.io")

	var resp *http.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = http.Get(bitnodesAPI)
		if err != nil {
			return nil, fmt.Errorf("HTTP GET failed: %w", err)
		}
		if resp.StatusCode == 200 {
			break
		}
		resp.Body.Close()
		if resp.StatusCode == 429 {
			backoff := time.Duration(30*(attempt+1)) * time.Second
			logger.Log.Warn().Int("attempt", attempt+1).Dur("backoff", backoff).Msg("Rate limited by bitnodes, retrying")
			time.Sleep(backoff)
			continue
		}
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed after retries, status: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var result struct {
		Nodes map[string][]interface{} `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("JSON decode failed: %w", err)
	}

	logger.Log.Info().Int("count", len(result.Nodes)).Msg("Retrieved nodes from bitnodes")

	// Collect all valid IPv4 nodes
	nodesByIP := make(map[string]*Node)
	var allIPs []string

	for addrPort, data := range result.Nodes {
		if len(data) < 5 {
			continue
		}

		// Parse address:port
		var addr string
		var port int
		if strings.HasPrefix(addrPort, "[") {
			continue // Skip IPv6
		}
		parts := strings.Split(addrPort, ":")
		if len(parts) != 2 {
			continue
		}
		addr = parts[0]
		fmt.Sscanf(parts[1], "%d", &port)

		// Skip .onion and non-IPv4
		if strings.HasSuffix(addr, ".onion") {
			continue
		}
		if net.ParseIP(addr) == nil || net.ParseIP(addr).To4() == nil {
			continue
		}

		node := &Node{Address: addr, Port: port}
		if v, ok := data[0].(float64); ok {
			node.Version = int(v)
		}
		if v, ok := data[1].(string); ok {
			node.UserAgent = v
		}

		nodesByIP[addr] = node
		allIPs = append(allIPs, addr)
	}

	logger.Log.Info().Int("count", len(allIPs)).Msg("Found IPv4 nodes, looking up geolocation")

	// Batch lookup geolocation (100 IPs per request)
	nodesByCountry := make(map[string][]*Node)
	batchSize := 100
	maxNodes := 1000
	nodesPerCountry := 10 // Keep 10 candidates per country for failover

	for i := 0; i < len(allIPs) && i < maxNodes; i += batchSize {
		end := i + batchSize
		if end > len(allIPs) {
			end = len(allIPs)
		}
		batch := allIPs[i:end]

		geoMap, err := lookupGeoBatch(batch)
		if err != nil {
			logger.Log.Warn().Err(err).Msg("Batch geo lookup failed")
			continue
		}

		for ip, geo := range geoMap {
			node := nodesByIP[ip]
			node.CountryCode = geo.CountryCode
			node.City = geo.City
			node.Latitude = geo.Lat
			node.Longitude = geo.Lon
			node.ASN = geo.AS
			node.OrgName = geo.Org

			// Only add if it's a target country and we don't have enough candidates
			if IsTargetCountry(node.CountryCode) && len(nodesByCountry[node.CountryCode]) < nodesPerCountry {
				nodesByCountry[node.CountryCode] = append(nodesByCountry[node.CountryCode], node)
			}
		}

		// Rate limit between batches
		time.Sleep(100 * time.Millisecond)
	}

	for country, nodes := range nodesByCountry {
		logger.Log.Info().Str("country", country).Int("count", len(nodes)).Msg("Found nodes")
	}

	return nodesByCountry, nil
}

// RefreshPeerPool fetches new nodes and updates the peer manager
func RefreshPeerPool(pm *PeerManager) {
	nodesByCountry, err := FetchNodes()
	if err != nil {
		logger.Log.Error().Err(err).Msg("Failed to fetch nodes")
		return
	}

	for country, nodes := range nodesByCountry {
		pm.SetAvailable(country, nodes)
	}
}

// StartDiscoveryRoutine starts periodic peer discovery
func StartDiscoveryRoutine(ctx context.Context, pm *PeerManager, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RefreshPeerPool(pm)
			}
		}
	}()
}
