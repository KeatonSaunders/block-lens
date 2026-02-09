import { useState, useEffect } from 'react';
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, Legend
} from 'recharts';
import {
  getPageRank, getCommunities, getStats, getCountryRankings,
  getHighRiskAddresses, getPropagationStats, traceFunds,
  getAddressMetrics, getAddressRisk
} from '../api';

const COLORS = ['#1a1a1a', '#d4622b', '#2563eb', '#059669', '#7c3aed', '#db2777', '#0891b2', '#65a30d'];

export default function Analytics() {
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState(null);
  const [lastRefresh, setLastRefresh] = useState(null);
  const [data, setData] = useState({
    stats: null,
    pagerank: [],
    communities: null,
    countries: [],
    highRisk: [],
    propagation: []
  });

  // Search state
  const [searchQuery, setSearchQuery] = useState('');
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchResult, setSearchResult] = useState(null);
  const [searchError, setSearchError] = useState(null);

  // Quick search helper
  const searchAddress = async (address) => {
    setSearchQuery(address);
    setSearchLoading(true);
    setSearchError(null);
    setSearchResult(null);

    try {
      const [metrics, risk] = await Promise.all([
        getAddressMetrics(address),
        getAddressRisk(address).catch(() => null),
      ]);
      setSearchResult({ metrics, risk });
    } catch (err) {
      setSearchError(err.message);
    } finally {
      setSearchLoading(false);
    }

    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  // Path finder state
  const [pathSource, setPathSource] = useState('');
  const [pathTarget, setPathTarget] = useState('');
  const [pathResult, setPathResult] = useState(null);
  const [pathLoading, setPathLoading] = useState(false);

  useEffect(() => {
    loadData();
    const interval = setInterval(() => loadData(), 30000);
    return () => clearInterval(interval);
  }, []);

  const loadData = async (isManual = false) => {
    if (isManual) setRefreshing(true);

    try {
      const [stats, pagerank, communities, countries, highRisk, propagation] = await Promise.all([
        getStats().catch(() => null),
        getPageRank(15).catch(() => ({ top_addresses: [] })),
        getCommunities().catch(() => null),
        getCountryRankings().catch(() => ({ rankings: [] })),
        getHighRiskAddresses(10).catch(() => ({ high_risk_addresses: [] })),
        getPropagationStats().catch(() => ({ by_region: [] }))
      ]);

      setData({
        stats,
        pagerank: pagerank.top_addresses || [],
        communities,
        countries: countries.rankings || [],
        highRisk: highRisk.high_risk_addresses || [],
        propagation: propagation.by_region || []
      });
      setLastRefresh(new Date());
      setError(null);
    } catch (err) {
      if (initialLoading) setError(err.message);
    } finally {
      setInitialLoading(false);
      setRefreshing(false);
    }
  };

  const handleSearch = async (e) => {
    e.preventDefault();
    if (!searchQuery.trim()) return;
    searchAddress(searchQuery.trim());
  };

  const handleFindPath = async (e) => {
    e.preventDefault();
    if (!pathSource || !pathTarget) return;

    setPathLoading(true);
    setPathResult(null);

    try {
      const result = await traceFunds(pathSource.trim(), pathTarget.trim(), 6);
      setPathResult(result);
    } catch (err) {
      setPathResult({ error: err.message });
    } finally {
      setPathLoading(false);
    }
  };

  if (initialLoading) {
    return (
      <div className="text-center py-24 text-gray-500">
        <div className="animate-pulse text-lg">Loading analytics...</div>
      </div>
    );
  }

  if (error && initialLoading) {
    return (
      <div className="text-center py-24">
        <div className="text-red-600 mb-4">{error}</div>
        <button onClick={() => loadData(true)} className="bg-black text-white px-6 py-2 hover:bg-gray-800 transition-colors">
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {/* Refresh indicator */}
      <div className="flex items-center justify-between text-sm text-gray-500">
        <span>
          {lastRefresh && (
            <>Updated {lastRefresh.toLocaleTimeString()} · Auto-refresh 30s</>
          )}
        </span>
        <button
          onClick={() => loadData(true)}
          disabled={refreshing}
          className="flex items-center gap-2 border border-black/20 hover:border-black/40 px-3 py-1.5 transition-colors disabled:opacity-50"
        >
          <span className={refreshing ? 'animate-spin' : ''}>↻</span>
          {refreshing ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      {/* Address Search */}
      <section className="border border-black/10 bg-white p-6">
        <h2 className="text-2xl font-display font-semibold mb-4">Address Search</h2>
        <form onSubmit={handleSearch} className="flex gap-3">
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Enter Bitcoin address (1..., 3..., or bc1...)"
            className="flex-1 border border-black/20 px-4 py-2.5 bg-transparent focus:outline-none focus:border-black font-mono text-sm"
          />
          <button
            type="submit"
            disabled={searchLoading}
            className="bg-black text-white px-6 py-2.5 font-medium hover:bg-gray-800 transition-colors disabled:opacity-50"
          >
            {searchLoading ? 'Searching...' : 'Search'}
          </button>
          {(searchResult || searchError || searchQuery) && (
            <button
              type="button"
              onClick={() => {
                setSearchQuery('');
                setSearchResult(null);
                setSearchError(null);
              }}
              className="border border-black/20 px-4 py-2.5 hover:bg-black/5 transition-colors"
            >
              Clear
            </button>
          )}
        </form>

        {searchError && (
          <div className="mt-4 p-4 bg-red-50 border border-red-200 text-red-700">
            {searchError}
          </div>
        )}

        {searchResult && (
          <div className="mt-6 space-y-4">
            <div className="border border-black/10 p-5">
              <h3 className="font-semibold text-lg mb-4 uppercase tracking-wide text-sm">Metrics</h3>
              <div className="grid grid-cols-2 md:grid-cols-5 gap-6">
                <div>
                  <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">Address</div>
                  <div className="font-mono text-xs break-all">{searchResult.metrics.address}</div>
                </div>
                <div>
                  <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">In-Degree</div>
                  <div className="text-3xl font-display">{searchResult.metrics.in_degree}</div>
                </div>
                <div>
                  <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">Out-Degree</div>
                  <div className="text-3xl font-display">{searchResult.metrics.out_degree}</div>
                </div>
                <div>
                  <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">Received</div>
                  <div className="text-2xl font-display">{(searchResult.metrics.total_received / 1e8).toFixed(4)}</div>
                  <div className="text-xs text-gray-500">BTC</div>
                </div>
                <div>
                  <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">Sent</div>
                  <div className="text-2xl font-display">{(searchResult.metrics.total_sent / 1e8).toFixed(4)}</div>
                  <div className="text-xs text-gray-500">BTC</div>
                </div>
              </div>
            </div>

            {searchResult.risk && (
              <div className="border border-black/10 p-5">
                <h3 className="font-semibold text-lg mb-4 uppercase tracking-wide text-sm">Risk Analysis</h3>
                <div className="flex items-start gap-6">
                  <div
                    className={`text-5xl font-display ${
                      searchResult.risk.score < 0.3
                        ? 'text-green-600'
                        : searchResult.risk.score < 0.7
                        ? 'text-amber-600'
                        : 'text-red-600'
                    }`}
                  >
                    {(searchResult.risk.score * 100).toFixed(0)}%
                  </div>
                  <div className="flex-1">
                    <div className="text-gray-600 mb-3">{searchResult.risk.explanation}</div>
                    <div className="flex flex-wrap gap-2">
                      {Object.entries(searchResult.risk.risk_factors).map(([factor, value]) => (
                        <span key={factor} className="border border-black/20 px-3 py-1 text-sm font-mono">
                          {factor}: {(value * 100).toFixed(0)}%
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
      </section>

      {/* Graph Stats Overview */}
      {data.stats && (
        <section className="border border-black/10 bg-white p-6">
          <h2 className="text-2xl font-display font-semibold mb-6">Graph Overview</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
            {[
              { label: 'Addresses', value: data.stats.nodes?.toLocaleString(), sub: 'nodes' },
              { label: 'Connections', value: data.stats.edges?.toLocaleString(), sub: 'edges' },
              { label: 'Density', value: (data.stats.density * 100).toFixed(4) + '%', sub: 'graph' },
              { label: 'Avg Degree', value: data.stats.avg_degree?.toFixed(2) || 'N/A', sub: 'connections' },
            ].map((stat, i) => (
              <div key={i} className="border-l-2 border-black pl-4">
                <div className="text-xs text-gray-500 uppercase tracking-wide mb-1">{stat.label}</div>
                <div className="text-3xl font-display">{stat.value}</div>
                <div className="text-xs text-gray-400">{stat.sub}</div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Two Column Layout */}
      <div className="grid md:grid-cols-2 gap-6">
        {/* PageRank */}
        <section className="border border-black/10 bg-white p-6">
          <h2 className="text-xl font-display font-semibold mb-2">PageRank</h2>
          <p className="text-sm text-gray-500 mb-4">Click address to search</p>
          {data.pagerank.length > 0 ? (
            <div className="space-y-2">
              {data.pagerank.slice(0, 10).map((item, i) => (
                <div key={i} className="flex items-center gap-3 group">
                  <span className="text-gray-400 w-5 font-mono text-sm">{i + 1}</span>
                  <button
                    onClick={() => searchAddress(item.address)}
                    className="font-mono text-sm text-black hover:underline truncate max-w-[180px]"
                    title={item.address}
                  >
                    {item.address}
                  </button>
                  <div className="flex-1 h-1.5 bg-gray-100">
                    <div
                      className="h-full bg-black"
                      style={{ width: `${(item.pagerank / data.pagerank[0].pagerank) * 100}%` }}
                    />
                  </div>
                  <span className="text-gray-500 font-mono text-xs w-16 text-right">
                    {item.pagerank.toFixed(5)}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-gray-400">No data</p>
          )}
        </section>

        {/* Communities */}
        <section className="border border-black/10 bg-white p-6">
          <h2 className="text-xl font-display font-semibold mb-2">Communities</h2>
          {data.communities && data.communities.communities?.length > 0 ? (
            <>
              <p className="text-sm text-gray-500 mb-4">
                {data.communities.total_communities} clusters detected
              </p>
              <div className="space-y-3 max-h-72 overflow-y-auto">
                {data.communities.communities.slice(0, 6).map((community, i) => (
                  <div key={i} className="border border-black/10 p-3">
                    <div className="flex items-center gap-2 mb-2">
                      <div
                        className="w-3 h-3"
                        style={{ backgroundColor: COLORS[i % COLORS.length] }}
                      />
                      <span className="font-medium text-sm">Cluster {community.id}</span>
                      <span className="text-gray-400 text-sm">· {community.size} addresses</span>
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {community.addresses.slice(0, 4).map((addr, j) => (
                        <button
                          key={j}
                          onClick={() => searchAddress(addr)}
                          className="font-mono text-xs bg-gray-100 hover:bg-gray-200 px-2 py-1 transition-colors"
                          title={addr}
                        >
                          {addr.slice(0, 8)}...
                        </button>
                      ))}
                      {community.addresses.length > 4 && (
                        <span className="text-gray-400 text-xs px-2 py-1">
                          +{community.addresses.length - 4}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="text-gray-400">No data</p>
          )}
        </section>
      </div>

      {/* Path Finder */}
      <section className="border border-black/10 bg-white p-6">
        <h2 className="text-2xl font-display font-semibold mb-4">Path Finder</h2>
        <form onSubmit={handleFindPath} className="flex flex-col md:flex-row gap-3 mb-4">
          <input
            type="text"
            value={pathSource}
            onChange={(e) => setPathSource(e.target.value)}
            placeholder="Source address"
            className="flex-1 border border-black/20 px-4 py-2.5 bg-transparent focus:outline-none focus:border-black font-mono text-sm"
          />
          <span className="text-gray-400 self-center font-display italic">to</span>
          <input
            type="text"
            value={pathTarget}
            onChange={(e) => setPathTarget(e.target.value)}
            placeholder="Target address"
            className="flex-1 border border-black/20 px-4 py-2.5 bg-transparent focus:outline-none focus:border-black font-mono text-sm"
          />
          <button
            type="submit"
            disabled={pathLoading}
            className="bg-black text-white px-6 py-2.5 font-medium hover:bg-gray-800 transition-colors disabled:opacity-50"
          >
            {pathLoading ? 'Finding...' : 'Find Path'}
          </button>
          {(pathSource || pathTarget || pathResult) && (
            <button
              type="button"
              onClick={() => {
                setPathSource('');
                setPathTarget('');
                setPathResult(null);
              }}
              className="border border-black/20 px-4 py-2.5 hover:bg-black/5 transition-colors"
            >
              Clear
            </button>
          )}
        </form>

        {pathResult && (
          <div className={`p-4 ${pathResult.error ? 'bg-red-50 border border-red-200' : 'border border-black/10'}`}>
            {pathResult.error ? (
              <p className="text-red-700">{pathResult.error}</p>
            ) : pathResult.path ? (
              <>
                <div className="text-green-700 mb-3 font-medium">
                  Path found · {pathResult.hops} hop{pathResult.hops !== 1 ? 's' : ''}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {pathResult.path.map((addr, i) => (
                    <span key={i} className="flex items-center gap-2">
                      <button
                        onClick={() => searchAddress(addr)}
                        className="bg-gray-100 hover:bg-gray-200 px-3 py-1.5 font-mono text-sm transition-colors"
                        title={addr}
                      >
                        {addr.slice(0, 10)}...
                      </button>
                      {i < pathResult.path.length - 1 && <span className="text-gray-400">→</span>}
                    </span>
                  ))}
                </div>
              </>
            ) : (
              <p className="text-amber-700">No path found between these addresses</p>
            )}
          </div>
        )}
      </section>

      {/* High Risk Addresses */}
      <section className="border border-black/10 bg-white p-6">
        <h2 className="text-2xl font-display font-semibold mb-2">High Risk Addresses</h2>
        <p className="text-sm text-gray-500 mb-4">Click address to search</p>
        {data.highRisk.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs uppercase tracking-wide text-gray-500 border-b border-black/10">
                  <th className="pb-3 font-medium">Address</th>
                  <th className="pb-3 font-medium">Risk</th>
                  <th className="pb-3 font-medium">PageRank</th>
                  <th className="pb-3 font-medium">Analysis</th>
                </tr>
              </thead>
              <tbody>
                {data.highRisk.map((item, i) => (
                  <tr key={i} className="border-b border-black/5 hover:bg-gray-50">
                    <td className="py-3">
                      <button
                        onClick={() => searchAddress(item.address)}
                        className="font-mono text-xs hover:underline"
                        title={item.address}
                      >
                        {item.address}
                      </button>
                    </td>
                    <td className="py-3">
                      <span className={`font-mono font-medium ${
                        item.risk_score > 0.7 ? 'text-red-600' :
                        item.risk_score > 0.4 ? 'text-amber-600' :
                        'text-green-600'
                      }`}>
                        {(item.risk_score * 100).toFixed(0)}%
                      </span>
                    </td>
                    <td className="py-3 text-gray-500 font-mono text-xs">{item.pagerank.toFixed(6)}</td>
                    <td className="py-3 text-gray-500 text-xs">{item.explanation}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-gray-400">No data</p>
        )}
      </section>

      {/* Two Column: Countries & Propagation */}
      <div className="grid md:grid-cols-2 gap-6">
        {/* Country Rankings */}
        <section className="border border-black/10 bg-white p-6">
          <h2 className="text-xl font-display font-semibold mb-4">First-Seen by Country</h2>
          {data.countries.length > 0 ? (
            <div className="space-y-3">
              {data.countries.slice(0, 10).map((country, i) => (
                <div key={i} className="flex items-center gap-3">
                  <span className="text-gray-500 font-mono text-sm w-6">{country.country_code}</span>
                  <div className="flex-1">
                    <div className="flex justify-between text-sm mb-1">
                      <span className="font-medium">{getCountryName(country.country_code)}</span>
                      <span className="text-gray-500 font-mono">{country.first_seen_count.toLocaleString()}</span>
                    </div>
                    <div className="h-1.5 bg-gray-100">
                      <div
                        className="h-full bg-black"
                        style={{ width: `${(country.first_seen_count / data.countries[0].first_seen_count) * 100}%` }}
                      />
                    </div>
                  </div>
                  <span className="text-gray-400 text-xs">{country.peer_count}p</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-gray-400">No data</p>
          )}
        </section>

        {/* Propagation Stats */}
        <section className="border border-black/10 bg-white p-6">
          <h2 className="text-xl font-display font-semibold mb-4">Propagation by Region</h2>
          {data.propagation.length > 0 ? (
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={groupByRegion(data.propagation)} margin={{ top: 10, right: 10, left: 10, bottom: 10 }}>
                  <XAxis dataKey="region" stroke="#999" tick={{ fill: '#666', fontSize: 12 }} axisLine={false} tickLine={false} />
                  <YAxis stroke="#999" tick={{ fill: '#666', fontSize: 12 }} axisLine={false} tickLine={false} domain={[0, 'auto']} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#fff', border: '1px solid #e5e5e5', borderRadius: 0 }}
                    formatter={(v, name) => [v.toLocaleString(), 'Observations']}
                  />
                  <Bar dataKey="observation_count" fill="#1a1a1a" />
                </BarChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <p className="text-gray-400">No data</p>
          )}
        </section>
      </div>
    </div>
  );
}

function countryFlag(code) {
  if (!code || code.length !== 2) return '🌍';
  const offset = 127397;
  return String.fromCodePoint(...[...code.toUpperCase()].map(c => c.charCodeAt(0) + offset));
}

const countryNames = {
  BR: 'Brazil', AR: 'Argentina', CL: 'Chile',
  ZA: 'South Africa', NG: 'Nigeria', KE: 'Kenya',
  US: 'United States', CA: 'Canada',
  DE: 'Germany', NL: 'Netherlands', RU: 'Russia',
  JP: 'Japan', SG: 'Singapore', IN: 'India', AE: 'UAE', MY: 'Malaysia', TH: 'Thailand', CN: 'China',
  AU: 'Australia', NZ: 'New Zealand',
  GB: 'United Kingdom', FR: 'France', ES: 'Spain', IT: 'Italy',
  KR: 'South Korea', HK: 'Hong Kong', ID: 'Indonesia', PH: 'Philippines',
};

const countryToRegion = {
  BR: 'S. America', AR: 'S. America', CL: 'S. America',
  ZA: 'Africa', NG: 'Africa', KE: 'Africa',
  US: 'N. America', CA: 'N. America',
  DE: 'Europe', NL: 'Europe', RU: 'Europe', GB: 'Europe', FR: 'Europe', ES: 'Europe', IT: 'Europe',
  JP: 'Asia', SG: 'Asia', IN: 'Asia', AE: 'Asia', MY: 'Asia', TH: 'Asia', CN: 'Asia', KR: 'Asia', HK: 'Asia', ID: 'Asia', PH: 'Asia',
  AU: 'Oceania', NZ: 'Oceania',
};

function getCountryName(code) {
  return countryNames[code] || code;
}

function groupByRegion(data) {
  const regionTotals = {};
  for (const item of data) {
    const region = countryToRegion[item.region] || item.region;
    if (!regionTotals[region]) {
      regionTotals[region] = { region, observation_count: 0 };
    }
    regionTotals[region].observation_count += item.observation_count;
  }
  return Object.values(regionTotals).sort((a, b) => b.observation_count - a.observation_count);
}
