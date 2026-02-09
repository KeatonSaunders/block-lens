import { useState, useEffect, useRef } from 'react';
import StatCard from './StatCard';
import WorldMap from './WorldMap';
import { getPrometheusMetrics, extractDashboardMetrics, getHealth, getGeoActivity, getPeerLocations, getObserverLocation } from '../api';

export default function Dashboard() {
  const [metrics, setMetrics] = useState(null);
  const [graphHealth, setGraphHealth] = useState(null);
  const [geoLocations, setGeoLocations] = useState([]);
  const [peerLocations, setPeerLocations] = useState([]);
  const [observerLocation, setObserverLocation] = useState(null);
  const [txPerSecond, setTxPerSecond] = useState(0);
  const lastTxCountRef = useRef(0);
  const lastFetchTimeRef = useRef(0);

  useEffect(() => {
    // Fetch metrics every 5 seconds
    const fetchMetrics = async () => {
      const raw = await getPrometheusMetrics();
      const extracted = extractDashboardMetrics(raw);
      if (extracted) {
        setMetrics(extracted);

        // Calculate tx/s using ref to avoid stale closure
        const currentTx = extracted.txReceived;
        const now = Date.now();
        if (lastTxCountRef.current > 0 && lastFetchTimeRef.current > 0) {
          const diff = currentTx - lastTxCountRef.current;
          const timeDelta = (now - lastFetchTimeRef.current) / 1000; // seconds
          if (timeDelta > 0) {
            setTxPerSecond(Math.max(0, diff / timeDelta));
          }
        }
        lastTxCountRef.current = currentTx;
        lastFetchTimeRef.current = now;
      }
    };

    // Fetch graph API health
    const fetchHealth = async () => {
      try {
        const health = await getHealth();
        setGraphHealth(health);
      } catch {
        setGraphHealth(null);
      }
    };

    // Fetch geo activity and peer locations
    const fetchGeoData = async () => {
      try {
        const [geoData, peerData] = await Promise.all([
          getGeoActivity(),
          getPeerLocations()
        ]);
        if (geoData.locations) {
          setGeoLocations(geoData.locations);
        }
        if (peerData.peers) {
          setPeerLocations(peerData.peers);
        }
      } catch {
        // Geo data not available
      }
    };

    // Fetch observer location (only once on mount)
    const fetchObserverLocation = async () => {
      try {
        const data = await getObserverLocation();
        if (data.lat && data.lng) {
          setObserverLocation(data);
        }
      } catch {
        // Observer location not available
      }
    };

    fetchMetrics();
    fetchHealth();
    fetchGeoData();
    fetchObserverLocation();

    // Quick second fetch after 1.5s to get initial tx/s reading
    const quickFetch = setTimeout(fetchMetrics, 1500);

    const metricsInterval = setInterval(fetchMetrics, 5000);
    const healthInterval = setInterval(fetchHealth, 30000);
    const geoInterval = setInterval(fetchGeoData, 10000);

    return () => {
      clearTimeout(quickFetch);
      clearInterval(metricsInterval);
      clearInterval(healthInterval);
      clearInterval(geoInterval);
    };
  }, []);

  if (!metrics) {
    return (
      <div className="text-center py-12 text-gray-500">
        <div className="animate-pulse text-lg">Loading metrics...</div>
        <p className="mt-2 text-sm">Make sure the observer is running on port 9090</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Main Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard
          title="Block Height"
          value={metrics.blockHeight}
          color="purple"
        />
        <StatCard
          title="Active Peers"
          value={metrics.peersActive}
          subtitle={`${metrics.peerConnections} total connections`}
          color="green"
        />
        <StatCard
          title="Transactions"
          value={metrics.txReceived}
          subtitle={`${txPerSecond.toFixed(1)} tx/s`}
          color="blue"
        />
        <StatCard
          title="Blocks Received"
          value={metrics.blocksReceived}
          color="yellow"
        />
      </div>

      {/* World Map - Transaction Activity */}
      <div className="bg-white border border-black/10 p-6 overflow-hidden">
        <h2 className="text-lg font-semibold mb-4">Global Network Activity</h2>
        <div className="h-[450px] overflow-hidden">
          <WorldMap
            locations={geoLocations}
            peers={peerLocations}
            txPerSecond={txPerSecond}
            observerLocation={observerLocation}
            activePeers={metrics?.peersActive || 0}
          />
        </div>
      </div>

      {/* Secondary Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard
          title="TX Announcements"
          value={metrics.invTxAnnouncements}
          color="blue"
        />
        <StatCard
          title="Deduplicated"
          value={metrics.txDeduplicated}
          subtitle="Saved network requests"
          color="green"
        />
        <StatCard
          title="Recorded to DB"
          value={metrics.txRecorded}
          color="purple"
        />
        <StatCard
          title="Disconnections"
          value={metrics.peerDisconnections}
          color="red"
        />
      </div>

      {/* Graph API Status */}
      <div className="bg-white border border-black/10 p-6">
        <h2 className="text-lg font-semibold mb-4">Graph Analytics API</h2>
        {graphHealth ? (
          <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
            <div>
              <span className="text-xs text-gray-500 uppercase tracking-wider">Status</span>
              <div className="text-emerald-600 font-medium mt-1">{graphHealth.status}</div>
            </div>
            <div>
              <span className="text-xs text-gray-500 uppercase tracking-wider">Graph Nodes</span>
              <div className="text-xl font-semibold font-mono mt-1">{graphHealth.nodes?.toLocaleString()}</div>
            </div>
            <div>
              <span className="text-xs text-gray-500 uppercase tracking-wider">Graph Edges</span>
              <div className="text-xl font-semibold font-mono mt-1">{graphHealth.edges?.toLocaleString()}</div>
            </div>
            <div>
              <span className="text-xs text-gray-500 uppercase tracking-wider">Version</span>
              <div className="font-medium mt-1">{graphHealth.version}</div>
            </div>
          </div>
        ) : (
          <p className="text-gray-500">Graph API not available. Start with: <code className="bg-gray-100 px-2 py-1 font-mono text-sm">python api.py</code></p>
        )}
      </div>
    </div>
  );
}
