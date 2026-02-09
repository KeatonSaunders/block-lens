# Bitcoin Network Intelligence Platform

Real-time Bitcoin network monitoring and transaction graph analytics. Connects directly to the Bitcoin P2P network to observe transaction propagation patterns, builds transaction graphs for flow analysis, and provides risk scoring based on network behavior.

> **Demo Project**: This is a demonstration/portfolio project. The database stores only recent transaction data (up to ~1 month rolling window), not the full Bitcoin blockchain history. Graph analytics and risk scores are computed on this limited dataset to showcase the methodology rather than provide production-grade coverage.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Bitcoin P2P Network                            │
│    ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐   │
│    │ DE  │  │ JP  │  │ BR  │  │ US  │  │ SG  │  │ AU  │  │ ZA  │  │ ... │   │
│    └──┬──┘  └──┬──┘  └──┬──┘  └──┬──┘  └──┬──┘  └──┬──┘  └──┬──┘  └──┬──┘   │
└───────┼────────┼────────┼────────┼────────┼────────┼────────┼────────┼──────┘
        │        │        │        │        │        │        │        │
        └────────┴────────┴────────┼────────┴────────┴────────┴────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │     btc-observer (Go)       │
                    │                             │
                    │  • P2P Protocol Handler     │
                    │  • Multi-node Connections   │
                    │  • Transaction Parsing      │
                    │  • Propagation Tracking     │
                    │  • Prometheus Metrics       │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │      PostgreSQL Database    │
                    │                             │
                    │  • Transactions & Blocks    │
                    │  • Inputs/Outputs (UTXO)    │
                    │  • Propagation Events       │
                    │  • Peer Geolocation         │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │  graph-analytics (Python)   │
                    │      (internal only)        │
                    │                             │
                    │  • Transaction Graph Build  │
                    │  • PageRank Centrality      │
                    │  • Community Detection      │
                    │  • Path Finding             │
                    │  • Risk Scoring             │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │   Caddy (reverse proxy +    │
                    │    automatic HTTPS)         │
                    │   (ports 80/443 — only      │
                    │    externally exposed)      │
                    │                             │
                    │  /        → React SPA       │
                    │  /api/*   → Analytics API   │
                    │  /metrics → Observer        │
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────▼──────────────┐
                    │   web-dashboard (React)     │
                    │                             │
                    │  • Real-time World Map      │
                    │  • Graph Visualizations     │
                    │  • Address Search           │
                    │  • Risk Analysis UI         │
                    └─────────────────────────────┘
```

> **Note on the API**: The analytics API currently runs as an internal service only, accessed by the frontend through the Caddy reverse proxy. It is not exposed to external clients. An external-facing API with authentication, rate limiting, and API key management can be added at a later stage.

## Key Features

### Network Observer (Go)
- **Direct P2P Connections**: Implements Bitcoin protocol (version handshake, inv/getdata, tx/block parsing)
- **Geo-Diverse Network**: Maintains 1 peer per target country across 17 countries (BR, AR, ZA, NG, KE, US, CA, DE, NL, RU, JP, SG, IN, AE, MY, TH, AU, NZ)
- **Transaction Propagation Tracking**: Records first-seen timestamps and origin peer for every transaction
- **Double-Spend Detection**: Identifies conflicting inputs across different transactions
- **Block Confirmation Tracking**: Links transactions to confirming blocks
- **Prometheus Metrics**: Exposes tx/s, peer counts, latency histograms

### Graph Analytics (Python/FastAPI)
- **Transaction Graph Construction**: Builds directed graph where nodes are addresses and edges are fund flows
- **PageRank Centrality**: Identifies influential addresses in the transaction network
- **Louvain Community Detection**: Clusters addresses that frequently transact together
- **Shortest Path Analysis**: Traces fund flows between any two addresses
- **Risk Scoring Model**: Evaluates addresses based on network behavior patterns

### Web Dashboard (React)
- **Real-Time World Map**: Visualizes transaction activity by geographic origin with live tx/s
- **Observer Metrics**: Peer count, block height, propagation stats by region
- **Address Search**: Look up any address for network metrics and risk analysis
- **Path Finder**: Find transaction paths between addresses
- **API Documentation**: Interactive endpoint reference

## Transaction Propagation Analysis

When a Bitcoin transaction is broadcast, it propagates through the P2P network. By connecting to geographically diverse nodes, we can observe:

1. **Origin Detection**: Which region first announces a transaction (proxy for broadcast origin)
2. **Propagation Velocity**: How quickly transactions spread across regions
3. **Network Topology**: Which nodes are well-connected vs. peripheral

```
Transaction Broadcast Timeline
──────────────────────────────
T+0ms     │ DE peer announces tx (first seen)
T+45ms    │ NL peer announces tx
T+120ms   │ US peer announces tx
T+180ms   │ SG peer announces tx
T+250ms   │ BR peer announces tx
          ▼
Analysis: Transaction likely originated in Europe
```

## Risk Scoring Methodology

The risk scoring model evaluates addresses based on observable network behavior:

| Factor | Weight | Description |
|--------|--------|-------------|
| **Double-Spend** | 45 | Address involved in detected double-spend attempt (direct evidence of malicious intent) |
| **Mixing Patterns** | 25 | Many inputs and outputs suggests mixing/tumbling service |
| **High Centrality** | 15 | PageRank score significantly above average indicates hub activity |
| **High Volume** | 10 | Transaction count exceeding typical wallet patterns |
| **Low Clustering** | 5 | Isolated transaction patterns (not connected to peer clusters) |

**Score Interpretation:**
- 0-25: Low risk - Normal wallet behavior
- 25-50: Medium risk - Elevated activity, warrants monitoring
- 50-75: High risk - Patterns consistent with mixing or high-volume services
- 75-100: Critical - Strong indicators of mixing/laundering patterns or double-spend involvement

### Limitations

This model effectively catches:
- **Double-spend attackers** — Direct evidence of malicious intent
- **Mixing services** — Distinctive many-to-many patterns

But it **cannot distinguish** between Coinbase and a mixing service—both have high PageRank, high volume, and many counterparties. It also misses one-time scammers, stolen fund recipients, and sophisticated actors who mimic normal patterns.

**What production systems add:** External address labels (sanctions lists, known services), taint analysis (hops from known bad addresses), entity clustering, and cross-chain tracking.

This platform demonstrates the methodology and infrastructure; production effectiveness requires labeled data.

**For detailed methodology, practical assessment, and academic references, see [RISK_MODEL.md](./RISK_MODEL.md).**

## Database Schema

```sql
-- Core transaction tracking
transactions              -- Parsed tx data (hash, fee, size, I/O counts)
transaction_inputs        -- Input references (prev_tx, prev_idx, value)
transaction_outputs       -- Output data (address, value, script)
transaction_observations  -- First-seen tracking (timestamp, peer, confirmations)

-- Propagation analysis
propagation_events        -- Per-peer announcement times for latency analysis
peer_connections          -- Peer metadata (version, services, geolocation)
blocks                    -- Block headers and confirmation data
```

**For detailed schema design rationale and indexing strategy, see [DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md).**

## API Endpoints

All endpoints are served internally via Caddy at `/api/*`. The API is not directly exposed to external clients.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/stats` | Graph statistics (nodes, edges, density) |
| GET | `/api/address/{addr}/metrics` | Address network metrics |
| GET | `/api/address/{addr}/risk` | Risk score and factors |
| GET | `/api/pagerank?top_n=10` | Top addresses by PageRank |
| GET | `/api/communities` | Detected address clusters |
| POST | `/api/path` | Find shortest path between addresses |
| GET | `/api/country-rankings` | First-seen counts by country |
| GET | `/api/propagation-stats` | Propagation timing by region |
| GET | `/api/high-risk-addresses` | Addresses with highest risk scores |
| GET | `/api/geo-activity` | Transaction activity by location (for map) |
| GET | `/api/peer-locations` | Connected peer locations |

## Quick Start

### Docker Compose (recommended)

```bash
# 1. Configure environment
cp .env.example .env
# Edit .env — set POSTGRES_PASSWORD and DOMAIN

# 2. Build and start all services
docker compose up --build -d

# 3. Access the dashboard
open https://yourdomain.com   # or http://localhost for local dev
```

This starts all four services (PostgreSQL, observer, analytics, dashboard) with Caddy as the single entry point. When `DOMAIN` is set to a real domain name, Caddy automatically provisions a Let's Encrypt TLS certificate. When set to `localhost` (the default), it serves plain HTTP for local development.

### Local Development

Prerequisites: Go 1.21+, Python 3.10+, PostgreSQL 14+, Node.js 18+

```bash
# 1. Database
createdb bitcoin_intel
psql bitcoin_intel < btc-observer/schema.sql

# 2. Observer
cd btc-observer/cmd/observer
cp config.example.json config.json  # Edit with your DB credentials
go build ./cmd/observer && ./observer

# 3. Analytics API
cd graph-analytics
python -m venv venv && source venv/bin/activate
pip install -r requirements.txt
python api.py

# 4. Dashboard
cd web-dashboard && npm install && npm run dev
```

Open http://localhost:5173 for local dev (Vite dev server)

## Project Structure

```
├── btc-observer/               # Go P2P network observer
│   ├── cmd/observer/           # Main entry point + config
│   ├── internal/
│   │   ├── protocol/           # Bitcoin P2P message parsing
│   │   ├── observer/           # Peer management, message handling
│   │   ├── database/           # PostgreSQL operations
│   │   ├── metrics/            # Prometheus instrumentation
│   │   └── logger/             # Structured logging (zerolog)
│   └── schema.sql              # Database schema
│
├── graph-analytics/            # Python analytics engine
│   ├── api.py                  # FastAPI REST server (internal, behind Caddy)
│   └── graph_analytics.py      # NetworkX graph operations
│
└── web-dashboard/              # React frontend
    └── src/
        ├── components/         # Dashboard, Analytics, WorldMap, ApiDocs
        └── api.js              # API client
```

## Design Decisions

### Why Geo-Diverse Peers?
Transaction propagation patterns reveal information about broadcast origin. A transaction first seen by a German node before propagating to others likely originated in Europe. This requires strategic peer selection across regions rather than random connections.

### Why NetworkX for Graph Analytics?
NetworkX provides battle-tested implementations of PageRank, community detection (Louvain), and path-finding algorithms. For the current scale (thousands of addresses), it offers excellent performance without the complexity of distributed graph databases.

### Why Separate Observer and Analytics?
- **Different update frequencies**: Observer writes continuously; analytics rebuilds periodically
- **Different scaling needs**: Observer is I/O bound (network); analytics is CPU bound (graph algorithms)
- **Language fit**: Go excels at concurrent network I/O; Python has superior graph/ML libraries

### Why Track First-Seen by Peer?
The peer that first announces a transaction is likely closer (in network topology) to the broadcast origin. Aggregating this data across geographic regions provides statistical signals about where transactions originate—valuable for compliance and forensics.

## Metrics & Observability

The observer exposes Prometheus metrics at `:9090/metrics`:

- `btc_transactions_received_total` - Total transactions observed
- `btc_blocks_received_total` - Total blocks received
- `btc_peers_active` - Currently connected peers
- `btc_peer_latency_ms` - Peer response latency histogram
- `btc_inv_tx_announcements_total` - Transaction announcements received
- `btc_tx_deduplicated_total` - Duplicate announcements filtered

## License

MIT
