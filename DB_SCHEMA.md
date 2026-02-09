# Database Schema: Bitcoin Intelligence Platform

This document describes the PostgreSQL database schema powering the Bitcoin Network Intelligence Platform, including table design rationale, relationship structure, data type choices, and indexing strategy.

## Table of Contents
- [Schema Overview](#schema-overview)
- [Table Designs](#table-designs)
- [Relationships and Data Flow](#relationships-and-data-flow)
- [Indexing Strategy](#indexing-strategy)
- [Data Type Rationale](#data-type-rationale)
- [Design Tradeoffs](#design-tradeoffs)

---

## Schema Overview

The schema is organized around two domains:

| Domain | Tables | Purpose |
|--------|--------|---------|
| **P2P Network Layer** | `peer_connections`, `propagation_events` | Track Bitcoin peers, their geolocation, and how transactions propagate across the network |
| **Blockchain Data** | `blocks`, `transactions`, `transaction_inputs`, `transaction_outputs`, `transaction_observations` | Store confirmed blockchain data and pre-confirmation observation metadata |

The schema captures data at two levels that most blockchain databases ignore: **pre-confirmation observation** (which peer announced a transaction first, propagation timing) and **network topology** (peer geolocation, connection statistics). These feed the graph analytics and risk scoring layers described in [RISK_MODEL.md](RISK_MODEL.md).

---

## Table Designs

### `peer_connections`

Tracks Bitcoin P2P network peers and their metadata.

```sql
peer_addr           VARCHAR(100) PRIMARY KEY
first_connected_at  TIMESTAMP NOT NULL
last_seen_at        TIMESTAMP
protocol_version    INT
user_agent          VARCHAR(200)
services            BIGINT
avg_latency_ms      INT
tx_announcements    INT DEFAULT 0
block_announcements INT DEFAULT 0
connection_count    INT DEFAULT 0
country_code        VARCHAR(2)
city                VARCHAR(100)
region              VARCHAR(50)
latitude            DECIMAL(9,6)
longitude           DECIMAL(9,6)
asn                 VARCHAR(100)
org_name            VARCHAR(200)
```

**Design rationale:** `peer_addr` (IP:port) is the natural primary key since each peer connection is uniquely identified by its network address. Geolocation fields are denormalized into this table rather than separated into a `geolocations` table because peer IPs are the only entities we geolocate, so a join table would add complexity without benefit. The `services` field uses `BIGINT` to store the Bitcoin protocol's 64-bit service flags bitmask natively.

### `blocks`

Stores block headers with propagation metadata.

```sql
block_hash      BYTEA PRIMARY KEY
height          INT UNIQUE NOT NULL
prev_block_hash BYTEA
merkle_root     BYTEA
timestamp       TIMESTAMP
difficulty      NUMERIC
nonce           BIGINT
tx_count        INT
first_seen_at   TIMESTAMP
first_peer_addr VARCHAR(100)
```

**Design rationale:** `block_hash` is the primary key because it is the canonical identifier in the Bitcoin protocol. `height` has a `UNIQUE` constraint because, while forks can produce multiple blocks at the same height, this platform stores only the accepted chain. `first_seen_at` and `first_peer_addr` capture which peer relayed the block first—data used for propagation analysis. `difficulty` uses `NUMERIC` (arbitrary precision) because Bitcoin difficulty values exceed the range of standard integer types.

### `transaction_observations`

Records pre-confirmation transaction metadata from P2P network observation.

```sql
tx_hash             BYTEA PRIMARY KEY
first_seen_at       TIMESTAMP NOT NULL
first_peer_addr     VARCHAR(100)
peer_count          INT DEFAULT 1
in_block_hash       BYTEA
confirmed_at        TIMESTAMP
replaced_by_tx      BYTEA
double_spend_flag   BOOLEAN DEFAULT FALSE
```

**Design rationale:** This table is deliberately separate from `transactions` because observation data exists before confirmation. A transaction can be observed in the mempool, flagged as a double-spend, and replaced—all before (or without ever) appearing in a block. The `double_spend_flag` and `replaced_by_tx` fields are critical for the risk model's highest-weighted factor (45 points). Keeping observations separate avoids nullable columns in the `transactions` table and preserves data for transactions that never confirm.

### `transactions`

Stores confirmed transaction metadata.

```sql
tx_hash         BYTEA PRIMARY KEY
block_hash      BYTEA REFERENCES blocks(block_hash)
block_height    INT
fee_satoshis    BIGINT
size_bytes      INT
weight          INT
input_count     INT
output_count    INT
total_input     BIGINT
total_output    BIGINT
```

**Design rationale:** `block_height` is denormalized from the `blocks` table for query convenience—many queries filter or sort by height without needing full block data. `fee_satoshis` is stored directly rather than computed from `total_input - total_output` to avoid repeated joins to inputs/outputs. `weight` (SegWit virtual size) is stored alongside `size_bytes` because fee rate calculations use weight units, not raw bytes.

### `transaction_inputs`

Stores transaction inputs with UTXO references.

```sql
tx_hash         BYTEA NOT NULL
input_index     INT NOT NULL
prev_tx_hash    BYTEA NOT NULL
prev_output_idx BIGINT NOT NULL
value_satoshis  BIGINT
script_sig      BYTEA
PRIMARY KEY (tx_hash, input_index)
```

**Design rationale:** The composite primary key `(tx_hash, input_index)` mirrors Bitcoin's own transaction structure where inputs are ordered within a transaction. `prev_tx_hash` and `prev_output_idx` form the outpoint reference that links to the spent UTXO—this is the core of Bitcoin's transaction chain and is essential for graph construction. `value_satoshis` is denormalized from the referenced output for query performance; without it, every input value lookup would require joining to `transaction_outputs`.

### `transaction_outputs`

Stores transaction outputs with UTXO spend tracking.

```sql
tx_hash         BYTEA NOT NULL
output_index    INT NOT NULL
address         VARCHAR(100)
value_satoshis  BIGINT NOT NULL
script_pubkey   BYTEA
spent_in_tx     BYTEA
spent_at        TIMESTAMP
PRIMARY KEY (tx_hash, output_index)
```

**Design rationale:** The composite primary key `(tx_hash, output_index)` matches Bitcoin's outpoint structure. `spent_in_tx` and `spent_at` track when an output is consumed, enabling UTXO set queries and spend-chain analysis. `address` is nullable because not all outputs have decodable addresses (e.g., OP_RETURN outputs, non-standard scripts). `script_pubkey` is stored as raw `BYTEA` to preserve the original locking script for any script type.

### `propagation_events`

Records per-peer transaction propagation timing.

```sql
id                  SERIAL PRIMARY KEY
tx_hash             BYTEA NOT NULL
peer_addr           VARCHAR(100) NOT NULL
announcement_time   TIMESTAMP NOT NULL
delay_from_first_ms INT
```

**Design rationale:** This is a high-volume append-only table—every transaction generates one row per observing peer. `SERIAL` is used as the primary key instead of `(tx_hash, peer_addr)` because the same peer could theoretically re-announce a transaction. `delay_from_first_ms` is precomputed (announcement_time minus the first observation) to avoid repeated timestamp arithmetic in queries. This table powers the geographic propagation analysis described in the risk model's future enhancements.

---

## Relationships and Data Flow

### Transaction Chain (UTXO Model)

```
transaction_outputs ──(spent_in_tx)──► transactions ──► transaction_inputs
        ▲                                                       │
        └──────────(prev_tx_hash, prev_output_idx)──────────────┘
```

This circular reference models Bitcoin's UTXO chain: outputs from one transaction become inputs to another. The graph analytics module traverses this structure to build the NetworkX directed graph used for PageRank, community detection, and risk scoring.

### Observation-to-Confirmation Flow

```
P2P Network
    │
    ▼
transaction_observations ──(first seen in mempool)
    │
    ▼
propagation_events ──(per-peer timing data)
    │
    ▼
transactions ──(confirmed in block)──► blocks
```

A transaction is first recorded in `transaction_observations` when seen in the mempool, accumulates `propagation_events` as peers relay it, and finally links to `transactions` when confirmed in a block.

### Peer-to-Data Links

```
peer_connections.peer_addr ◄── transaction_observations.first_peer_addr
peer_connections.peer_addr ◄── propagation_events.peer_addr
peer_connections.peer_addr ◄── blocks.first_peer_addr
```

These are intentionally not enforced as foreign keys. Peers can disconnect and be removed while their historical observation data remains valuable. Enforcing referential integrity here would force a choice between losing observation data or keeping stale peer records.

---

## Indexing Strategy

### Index Summary

| Index | Table | Column(s) | Type | Purpose |
|-------|-------|-----------|------|---------|
| `idx_peer_region` | `peer_connections` | `region` | B-tree | Filter peers by geographic region for propagation analysis |
| `idx_blocks_height` | `blocks` | `height` | B-tree | Block lookup by number—the most common block query pattern |
| `idx_blocks_timestamp` | `blocks` | `timestamp` | B-tree | Time-range queries for block production analysis |
| `idx_tx_obs_first_seen` | `transaction_observations` | `first_seen_at` | B-tree | Time-range queries on mempool observations |
| `idx_tx_obs_unconfirmed` | `transaction_observations` | `in_block_hash` | Partial | Mempool monitoring—only indexes rows where `in_block_hash IS NULL` |
| `idx_transactions_block` | `transactions` | `block_hash` | B-tree | Group transactions by block—used when loading block contents |
| `idx_tx_inputs_address` | `transaction_inputs` | `address` | B-tree | Address-based transaction history lookups |
| `idx_tx_inputs_prev_outpoint` | `transaction_inputs` | `(prev_tx_hash, prev_output_idx)` | Composite B-tree | UTXO chain traversal—trace funds backward through the graph |
| `idx_tx_outputs_address` | `transaction_outputs` | `address` | B-tree | Address-based balance and history queries |
| `idx_tx_outputs_utxo` | `transaction_outputs` | `spent_in_tx` | Partial | UTXO set queries—only indexes unspent outputs (`spent_in_tx IS NULL`) |
| `idx_propagation_tx` | `propagation_events` | `tx_hash` | B-tree | Retrieve all propagation events for a specific transaction |

### Why Partial Indexes

Two indexes use PostgreSQL's `WHERE` clause to create partial indexes. This is a deliberate choice:

**`idx_tx_obs_unconfirmed`** — Only indexes transactions where `in_block_hash IS NULL` (unconfirmed). The mempool is a small fraction of all historical transactions. Queries like "show me pending transactions" only need to scan the unconfirmed subset, so indexing the entire table wastes space and write I/O. As transactions confirm, they drop out of this index automatically.

**`idx_tx_outputs_utxo`** — Only indexes outputs where `spent_in_tx IS NULL` (unspent). The UTXO set is a core concept in Bitcoin—the set of all currently spendable outputs. At any point, the majority of historical outputs have been spent. A full index would be dominated by spent outputs that UTXO queries never need. This keeps the index small and fast for balance lookups and UTXO set analysis.

### Why Composite Indexes

**`idx_tx_inputs_prev_outpoint`** on `(prev_tx_hash, prev_output_idx)` — This composite index supports UTXO chain traversal. The query "which transaction spent this specific output?" requires matching both the previous transaction hash and the output index within that transaction. A single-column index on `prev_tx_hash` would narrow the search but still require scanning all inputs from that transaction. The composite index resolves to a single row directly, which matters for graph construction where millions of these lookups occur during the periodic rebuild cycle.

### Indexes Not Created (and Why)

| Potential Index | Why Omitted |
|-----------------|-------------|
| `propagation_events(peer_addr)` | Per-peer propagation queries are rare; most analysis groups by `tx_hash` |
| `blocks(prev_block_hash)` | Chain traversal uses `height` (sequential), not `prev_block_hash` lookups |
| `transactions(block_height)` | `block_hash` index covers block-based grouping; height queries go through `blocks` table first |
| `transaction_observations(double_spend_flag)` | Boolean columns have extremely low selectivity; a sequential scan filtered by the unconfirmed partial index is faster |
| `peer_connections(country_code)` | Table is small (hundreds to low thousands of rows); sequential scan is fast enough |

---

## Data Type Rationale

| Data Type | Used For | Why |
|-----------|----------|-----|
| `BYTEA` | Transaction hashes, block hashes, merkle roots, script data | Raw 32-byte binary is half the storage of hex-encoded `VARCHAR(64)` and avoids encoding/decoding overhead. Hash comparisons on binary are faster than string comparisons |
| `VARCHAR(100)` for addresses | Bitcoin addresses | Accommodates all address formats: legacy P2PKH (34 chars), P2SH (34 chars), Bech32/SegWit (42-62 chars), and Taproot Bech32m (62 chars) with headroom for future formats |
| `BIGINT` for satoshis | `value_satoshis`, `fee_satoshis`, `total_input`, `total_output` | Bitcoin's maximum supply is 2,100,000,000,000,000 satoshis (~2.1 × 10^15), which fits within `BIGINT`'s range (9.2 × 10^18). `INT` would overflow at ~21 BTC |
| `NUMERIC` for difficulty | `blocks.difficulty` | Bitcoin difficulty values can exceed `BIGINT` range. `NUMERIC` provides arbitrary precision at the cost of slower arithmetic, which is acceptable for a rarely-computed field |
| `DECIMAL(9,6)` | Latitude, longitude | Six decimal places gives ~0.1 meter precision—more than sufficient for city-level geolocation of peers, while keeping storage compact |
| `BIGINT` for services | `peer_connections.services` | Bitcoin protocol service flags are a 64-bit bitmask; `BIGINT` stores all 64 bits natively |
| `TIMESTAMP` | All time fields | Microsecond precision for propagation timing analysis. Stored without timezone—all times are UTC by convention |

---

## Design Tradeoffs

### Denormalization Choices

Several fields are intentionally denormalized:

1. **`transactions.block_height`** — Duplicates data derivable from joining to `blocks`. Justified because block height is used in nearly every transaction query and eliminating the join has measurable impact at scale.

2. **`transaction_inputs.value_satoshis`** — Duplicates the value from the referenced output. Without this, computing input values requires joining `transaction_inputs` to `transaction_outputs` on `(prev_tx_hash, prev_output_idx)` for every input—an expensive operation during graph construction where millions of inputs are processed.

3. **`propagation_events.delay_from_first_ms`** — Precomputed from `announcement_time - first_seen_at`. Avoids repeated timestamp arithmetic in aggregation queries over this high-volume table.

### What's Not in the Schema

1. **No address table** — Addresses are stored as columns in inputs/outputs rather than having their own entity table. This avoids an extra join for the most common queries, at the cost of duplicated address strings. For entity resolution and clustering (grouping addresses by owner), a separate `entities` or `address_clusters` table would be needed.

2. **No foreign keys on peer references** — `first_peer_addr` columns in `blocks` and `transaction_observations` don't reference `peer_connections`. This is intentional: peers are transient, but observation data is permanent. Enforcing referential integrity would either prevent peer cleanup or require cascading deletes that destroy historical data.

3. **No partitioning** — Tables are not partitioned by time or block height. For the current scale (single observer node), this is fine. At production scale with years of data, partitioning `transaction_inputs`, `transaction_outputs`, and `propagation_events` by block height ranges would improve query performance and enable efficient data archival.

---

*This schema is designed for a single-observer deployment. Scaling to multiple observer nodes would require additional schema considerations: conflict resolution for `first_seen_at` timestamps, deduplication of propagation events, and potentially a distributed database architecture.*
