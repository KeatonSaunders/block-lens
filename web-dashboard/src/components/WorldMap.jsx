import { memo } from 'react';
import {
  ComposableMap,
  Geographies,
  Geography,
  Marker,
  Line,
} from 'react-simple-maps';

const geoUrl = 'https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json';

function WorldMap({ locations = [], peers = [], txPerSecond = 0, observerLocation = null, activePeers = 0 }) {
  // Find max tx_count for scaling markers
  const maxTx = Math.max(...locations.map((l) => l.tx_count), 1);

  // Default to center of map if no observer location
  const observer = observerLocation || { lat: 0, lng: 0 };

  return (
    <div className="relative w-full h-full overflow-hidden">
      {/* TX/s overlay */}
      <div className="absolute top-3 left-3 z-10 bg-white/95 border border-black/10 px-4 py-3">
        <div className="text-xs text-gray-500 uppercase tracking-wider">Live</div>
        <div className="text-3xl font-mono font-semibold">{txPerSecond.toFixed(1)}</div>
        <div className="text-xs text-gray-500">tx/sec</div>
      </div>

      {/* Legend */}
      <div className="absolute top-3 right-3 z-10 bg-white/95 border border-black/10 px-4 py-3">
        <div className="text-xs text-gray-500 uppercase tracking-wider mb-2">Legend</div>
        <div className="flex flex-col gap-1.5 text-xs">
          <div className="flex items-center gap-2">
            <span className="w-3 h-3 rounded-full bg-black/50"></span>
            <span className="text-gray-600">Node</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-3 h-3 rounded-full bg-[#d4622b]"></span>
            <span className="text-gray-600">Observer</span>
          </div>
        </div>
      </div>

      {/* Peer count */}
      <div className="absolute bottom-3 left-3 z-10 bg-white/95 border border-black/10 px-4 py-2">
        <div className="text-xs text-gray-500 uppercase tracking-wider">Active Peers</div>
        <div className="text-xl font-mono font-semibold">{activePeers}</div>
      </div>

      <ComposableMap
        projection="geoMercator"
        projectionConfig={{
          scale: 130,
          center: [10, 30],
        }}
        width={900}
        height={450}
        style={{ width: '100%', height: '100%' }}
      >
        {/* World geography - filter out Antarctica */}
        <Geographies geography={geoUrl}>
          {({ geographies }) =>
            geographies
              .filter((geo) => geo.properties.name !== 'Antarctica')
              .map((geo) => (
                <Geography
                  key={geo.rsmKey}
                  geography={geo}
                  fill="#e8e8e8"
                  stroke="#d0d0d0"
                  strokeWidth={0.5}
                  style={{
                    default: { outline: 'none' },
                    hover: { outline: 'none', fill: '#ddd' },
                    pressed: { outline: 'none' },
                  }}
                />
              ))
          }
        </Geographies>

        {/* Network lines from observer to each peer */}
        {observerLocation && peers.map((peer, idx) => (
          <Line
            key={`line-${idx}`}
            from={[observer.lng, observer.lat]}
            to={[peer.lng, peer.lat]}
            stroke="rgba(0, 0, 0, 0.12)"
            strokeWidth={1}
            strokeLinecap="round"
          />
        ))}

        {/* Peer markers (small dots) */}
        {peers.map((peer, idx) => (
          <Marker key={`peer-${idx}`} coordinates={[peer.lng, peer.lat]}>
            <circle
              r={peer.active ? 4 : 3}
              fill={peer.active ? "rgba(0, 0, 0, 0.6)" : "rgba(0, 0, 0, 0.3)"}
              stroke="#fff"
              strokeWidth={0.5}
            />
            <title>
              {peer.city || peer.country_code}: {peer.peer_count} peer(s) {peer.active ? '(active)' : ''}
            </title>
          </Marker>
        ))}

        {/* Transaction activity markers (larger, scaled by activity) */}
        {locations.map((location, idx) => {
          const size = Math.max(5, Math.min(18, (location.tx_count / maxTx) * 18 + 5));
          const opacity = Math.max(0.4, Math.min(0.85, (location.tx_count / maxTx) * 0.45 + 0.4));

          return (
            <Marker key={`tx-${idx}`} coordinates={[location.lng, location.lat]}>
              <circle
                r={size}
                fill={`rgba(26, 26, 26, ${opacity})`}
                stroke="#1a1a1a"
                strokeWidth={0.5}
              />
              <title>
                {location.country_code}: {location.tx_count} transactions (1h)
              </title>
            </Marker>
          );
        })}

        {/* Observer location marker */}
        {observerLocation && (
          <Marker coordinates={[observer.lng, observer.lat]}>
            <circle r={6} fill="#d4622b" stroke="#fff" strokeWidth={2} />
            <title>Your Observer ({observerLocation.city || 'Unknown'})</title>
          </Marker>
        )}
      </ComposableMap>
    </div>
  );
}

export default memo(WorldMap);
