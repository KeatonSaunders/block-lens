import { useState } from 'react';

const API_BASE = '/api';

const endpoints = [
  {
    category: 'Health & Stats',
    items: [
      {
        method: 'GET',
        path: '/',
        description: 'Health check - returns API status and graph size',
        example: { status: 'online', nodes: 1234, edges: 5678, version: '0.1.0' }
      },
      {
        method: 'GET',
        path: '/health',
        description: 'Health check for monitoring and load balancers',
        example: { status: 'healthy', graph_loaded: true }
      },
      {
        method: 'GET',
        path: '/stats',
        description: 'Get graph statistics including density and average degree',
        example: { nodes: 1234, edges: 5678, density: 0.0012, avg_degree: 2.5 }
      }
    ]
  },
  {
    category: 'Address Analysis',
    items: [
      {
        method: 'GET',
        path: '/address/{address}/metrics',
        description: 'Get network metrics for a Bitcoin address',
        params: [{ name: 'address', type: 'string', description: 'Bitcoin address (1..., 3..., or bc1...)' }],
        example: {
          address: '1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa',
          in_degree: 5,
          out_degree: 2,
          total_received: 5000000000,
          total_sent: 1000000000,
          clustering_coefficient: 0.25
        }
      },
      {
        method: 'GET',
        path: '/address/{address}/risk',
        description: 'Calculate risk score for an address based on network behavior',
        params: [{ name: 'address', type: 'string', description: 'Bitcoin address' }],
        example: {
          address: '1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa',
          score: 0.35,
          risk_factors: { high_velocity: 0.2, unusual_patterns: 0.15 },
          explanation: 'Moderate activity with some unusual patterns'
        }
      }
    ]
  },
  {
    category: 'Graph Analytics',
    items: [
      {
        method: 'GET',
        path: '/pagerank',
        description: 'Get top addresses ranked by PageRank score',
        params: [{ name: 'top_n', type: 'int', description: 'Number of results (default: 10)' }],
        example: {
          top_addresses: [
            { address: '1A1zP1...', pagerank: 0.00123 },
            { address: '3J98t1...', pagerank: 0.00098 }
          ]
        }
      },
      {
        method: 'GET',
        path: '/communities',
        description: 'Detect address communities/clusters using graph algorithms',
        example: {
          total_communities: 15,
          communities: [
            { id: 0, size: 45, addresses: ['1A1zP1...', '3J98t1...'] }
          ]
        }
      },
      {
        method: 'POST',
        path: '/path',
        description: 'Find shortest path between two addresses',
        body: { source: 'string', target: 'string', max_hops: 'int (default: 5)' },
        example: {
          source: '1A1zP1...',
          target: '3J98t1...',
          path: ['1A1zP1...', '1BvBMSE...', '3J98t1...'],
          hops: 2
        }
      },
      {
        method: 'GET',
        path: '/high-risk-addresses',
        description: 'Get addresses with highest risk scores',
        params: [{ name: 'top_n', type: 'int', description: 'Number of results (default: 10)' }],
        example: {
          high_risk_addresses: [
            { address: '1Bad...', risk_score: 0.85, pagerank: 0.001, explanation: 'High velocity mixing' }
          ]
        }
      }
    ]
  },
  {
    category: 'Network Intelligence',
    items: [
      {
        method: 'GET',
        path: '/country-rankings',
        description: 'Countries ranked by first-seen transaction observations',
        example: {
          rankings: [
            { country_code: 'US', region: 'na_east', first_seen_count: 1234, peer_count: 2 }
          ]
        }
      },
      {
        method: 'GET',
        path: '/propagation-stats',
        description: 'Transaction propagation statistics by region',
        example: {
          by_region: [
            { region: 'europe', observation_count: 5000, avg_delay_ms: 150, min_delay_ms: 0, max_delay_ms: 2000 }
          ]
        }
      }
    ]
  }
];

export default function ApiDocs() {
  const [expandedEndpoint, setExpandedEndpoint] = useState(null);

  const methodColors = {
    GET: 'bg-emerald-50 text-emerald-700 border-emerald-300',
    POST: 'bg-blue-50 text-blue-700 border-blue-300',
    DELETE: 'bg-red-50 text-red-700 border-red-300'
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="bg-white border border-black/10 p-6">
        <h2 className="text-xl font-semibold mb-3">API Documentation</h2>
        <p className="text-gray-600 mb-4">
          Base URL: <code className="bg-gray-100 px-2 py-1 font-mono text-sm">{API_BASE}</code>
        </p>
        <p className="text-gray-500 text-sm">
          The API is served internally through the Caddy reverse proxy. All endpoints are unauthenticated.
          An external-facing API with authentication and rate limiting can be added at a later stage.
        </p>
      </div>

      {/* Endpoints by category */}
      {endpoints.map((category, catIdx) => (
        <div key={catIdx} className="bg-white border border-black/10 p-6">
          <h3 className="text-lg font-semibold mb-4">{category.category}</h3>
          <div className="space-y-3">
            {category.items.map((endpoint, idx) => {
              const key = `${catIdx}-${idx}`;
              const isExpanded = expandedEndpoint === key;

              return (
                <div key={idx} className="border border-black/10 overflow-hidden">
                  <button
                    onClick={() => setExpandedEndpoint(isExpanded ? null : key)}
                    className="w-full flex items-center gap-3 p-4 hover:bg-gray-50 text-left"
                  >
                    <span className={`px-2 py-1 border text-xs font-bold ${methodColors[endpoint.method]}`}>
                      {endpoint.method}
                    </span>
                    <code className="text-gray-800 font-mono text-sm">{endpoint.path}</code>
                    <span className="flex-1 text-gray-500 text-sm truncate">{endpoint.description}</span>
                    <span className="text-gray-400">{isExpanded ? '−' : '+'}</span>
                  </button>

                  {isExpanded && (
                    <div className="border-t border-black/10 p-4 bg-gray-50 space-y-4">
                      <p className="text-gray-700">{endpoint.description}</p>

                      {endpoint.params && (
                        <div>
                          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Parameters</h4>
                          <table className="w-full text-sm">
                            <thead>
                              <tr className="text-left text-gray-500 border-b border-black/5">
                                <th className="pb-2">Name</th>
                                <th className="pb-2">Type</th>
                                <th className="pb-2">Description</th>
                              </tr>
                            </thead>
                            <tbody>
                              {endpoint.params.map((param, pIdx) => (
                                <tr key={pIdx} className="text-gray-700">
                                  <td className="py-2"><code className="font-mono text-sm">{param.name}</code></td>
                                  <td className="py-2 text-gray-500">{param.type}</td>
                                  <td className="py-2 text-gray-500">{param.description}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}

                      {endpoint.body && (
                        <div>
                          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Request Body</h4>
                          <pre className="bg-white border border-black/10 p-3 text-sm overflow-x-auto font-mono">
                            {JSON.stringify(endpoint.body, null, 2)}
                          </pre>
                        </div>
                      )}

                      {endpoint.example && (
                        <div>
                          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Example Response</h4>
                          <pre className="bg-white border border-black/10 p-3 text-sm overflow-x-auto font-mono text-emerald-700">
                            {JSON.stringify(endpoint.example, null, 2)}
                          </pre>
                        </div>
                      )}

                      <div>
                        <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Try it</h4>
                        <code className="block bg-white border border-black/10 p-3 text-sm text-gray-700 overflow-x-auto font-mono">
                          curl {endpoint.method !== 'GET' ? `-X ${endpoint.method} ` : ''}
                          {endpoint.body ? `-H "Content-Type: application/json" -d '${JSON.stringify(endpoint.body)}' ` : ''}
                          {API_BASE}{endpoint.path.replace('{address}', '1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa')}
                        </code>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
