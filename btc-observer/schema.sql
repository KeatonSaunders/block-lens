-- Bitcoin Intelligence Platform - PostgreSQL Schema

CREATE TABLE IF NOT EXISTS peer_connections (
    peer_addr           VARCHAR(100) PRIMARY KEY,
    first_connected_at  TIMESTAMP NOT NULL,
    last_seen_at        TIMESTAMP,
    protocol_version    INT,
    user_agent          VARCHAR(200),
    services            BIGINT,
    avg_latency_ms      INT,
    tx_announcements    INT DEFAULT 0,
    block_announcements INT DEFAULT 0,
    connection_count    INT DEFAULT 0,
    -- Geolocation fields
    country_code        VARCHAR(2),
    city                VARCHAR(100),
    region              VARCHAR(50),
    latitude            DECIMAL(9,6),
    longitude           DECIMAL(9,6),
    asn                 VARCHAR(100),
    org_name            VARCHAR(200)
);

CREATE INDEX IF NOT EXISTS idx_peer_region ON peer_connections(region);

CREATE TABLE IF NOT EXISTS blocks (
    block_hash      BYTEA PRIMARY KEY,
    height          INT UNIQUE NOT NULL,
    prev_block_hash BYTEA,
    merkle_root     BYTEA,
    timestamp       TIMESTAMP,
    difficulty      NUMERIC,
    nonce           BIGINT,
    tx_count        INT,
    first_seen_at   TIMESTAMP,
    first_peer_addr VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_blocks_height ON blocks(height);
CREATE INDEX IF NOT EXISTS idx_blocks_timestamp ON blocks(timestamp);

CREATE TABLE IF NOT EXISTS transaction_observations (
    tx_hash             BYTEA PRIMARY KEY,
    first_seen_at       TIMESTAMP NOT NULL,
    first_peer_addr     VARCHAR(100),
    peer_count          INT DEFAULT 1,
    in_block_hash       BYTEA,
    confirmed_at        TIMESTAMP,
    replaced_by_tx      BYTEA,
    double_spend_flag   BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_tx_obs_first_seen ON transaction_observations(first_seen_at);
CREATE INDEX IF NOT EXISTS idx_tx_obs_unconfirmed ON transaction_observations(in_block_hash)
    WHERE in_block_hash IS NULL;

CREATE TABLE IF NOT EXISTS transactions (
    tx_hash         BYTEA PRIMARY KEY,
    block_hash      BYTEA REFERENCES blocks(block_hash),
    block_height    INT,
    fee_satoshis    BIGINT,
    size_bytes      INT,
    weight          INT,
    input_count     INT,
    output_count    INT,
    total_input     BIGINT,
    total_output    BIGINT
);

CREATE INDEX IF NOT EXISTS idx_transactions_block ON transactions(block_hash);

CREATE TABLE IF NOT EXISTS transaction_inputs (
    tx_hash         BYTEA NOT NULL,
    input_index     INT NOT NULL,
    prev_tx_hash    BYTEA NOT NULL,
    prev_output_idx BIGINT NOT NULL,
    value_satoshis  BIGINT,
    script_sig      BYTEA,
    address         VARCHAR(100),
    PRIMARY KEY (tx_hash, input_index)
);

CREATE INDEX IF NOT EXISTS idx_tx_inputs_address ON transaction_inputs(address);
CREATE INDEX IF NOT EXISTS idx_tx_inputs_prev_outpoint ON transaction_inputs(prev_tx_hash, prev_output_idx);

CREATE TABLE IF NOT EXISTS transaction_outputs (
    tx_hash         BYTEA NOT NULL,
    output_index    INT NOT NULL,
    address         VARCHAR(100),
    value_satoshis  BIGINT NOT NULL,
    script_pubkey   BYTEA,
    spent_in_tx     BYTEA,
    spent_at        TIMESTAMP,
    PRIMARY KEY (tx_hash, output_index)
);

CREATE INDEX IF NOT EXISTS idx_tx_outputs_address ON transaction_outputs(address);
CREATE INDEX IF NOT EXISTS idx_tx_outputs_utxo ON transaction_outputs(spent_in_tx)
    WHERE spent_in_tx IS NULL;

CREATE TABLE IF NOT EXISTS propagation_events (
    id                  SERIAL PRIMARY KEY,
    tx_hash             BYTEA NOT NULL,
    peer_addr           VARCHAR(100) NOT NULL,
    announcement_time   TIMESTAMP NOT NULL,
    delay_from_first_ms INT
);

CREATE INDEX IF NOT EXISTS idx_propagation_tx ON propagation_events(tx_hash);
