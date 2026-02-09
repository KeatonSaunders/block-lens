async function fetchApi(path, options = {}) {
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  const response = await fetch(`/api${path}`, { ...options, headers });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ detail: response.statusText }));
    throw new Error(error.detail || 'API request failed');
  }

  return response.json();
}

// Health & Stats
export async function getHealth() {
  return fetchApi('/');
}

export async function getStats() {
  return fetchApi('/stats');
}

// Address endpoints
export async function getAddressMetrics(address) {
  return fetchApi(`/address/${address}/metrics`);
}

export async function getAddressRisk(address) {
  return fetchApi(`/address/${address}/risk`);
}

// Graph analytics
export async function getPageRank(topN = 10) {
  return fetchApi(`/pagerank?top_n=${topN}`);
}

export async function getCommunities() {
  return fetchApi('/communities');
}

export async function traceFunds(source, target, maxHops = 5) {
  return fetchApi('/path', {
    method: 'POST',
    body: JSON.stringify({ source, target, max_hops: maxHops }),
  });
}

// Analytics endpoints
export async function getCountryRankings() {
  return fetchApi('/country-rankings');
}

export async function getHighRiskAddresses(topN = 10) {
  return fetchApi(`/high-risk-addresses?top_n=${topN}`);
}

export async function getPropagationStats() {
  return fetchApi('/propagation-stats');
}

export async function getGeoActivity() {
  return fetchApi('/geo-activity');
}

export async function getPeerLocations() {
  return fetchApi('/peer-locations');
}

export async function getObserverLocation() {
  return fetchApi('/observer-location');
}

// Prometheus metrics (parse text format)
export async function getPrometheusMetrics() {
  try {
    const response = await fetch('/metrics/metrics');
    const text = await response.text();
    return parsePrometheusMetrics(text);
  } catch (error) {
    console.error('Failed to fetch metrics:', error);
    return null;
  }
}

function parsePrometheusMetrics(text) {
  const metrics = {};
  const lines = text.split('\n');

  for (const line of lines) {
    if (line.startsWith('#') || !line.trim()) continue;

    const match = line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)\s*(\{[^}]*\})?\s+(.+)$/);
    if (match) {
      const [, name, labels, value] = match;
      const key = labels ? `${name}${labels}` : name;
      metrics[key] = parseFloat(value);
    }
  }

  return metrics;
}

export function extractDashboardMetrics(raw) {
  if (!raw) return null;

  return {
    txReceived: raw['btc_transactions_received_total'] || 0,
    txRecorded: raw['btc_transactions_recorded_total'] || 0,
    blocksReceived: raw['btc_blocks_received_total'] || 0,
    blockHeight: raw['btc_block_height'] || 0,
    peersActive: raw['btc_peers_active'] || 0,
    peerConnections: raw['btc_peer_connections_total'] || 0,
    peerDisconnections: raw['btc_peer_disconnections_total'] || 0,
    invTxAnnouncements: raw['btc_inv_tx_announcements_total'] || 0,
    txDeduplicated: raw['btc_tx_deduplicated_total'] || 0,
  };
}
