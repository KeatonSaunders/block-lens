"""
REST API for Bitcoin Transaction Graph Analytics

Internal API — served behind Caddy, not exposed to external clients directly.
"""

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Optional, Dict
from datetime import datetime

from graph_analytics import TransactionGraph, RiskAnalyzer, RiskScore, get_db_connection

app = FastAPI(
    title="Bitcoin Graph Analytics API",
    description="Analyze Bitcoin transaction networks and calculate risk scores",
    version="0.1.0"
)

# Global graph instance (in production, use caching/background jobs)
transaction_graph = TransactionGraph()
risk_analyzer = None


class AddressMetricsResponse(BaseModel):
    address: str
    in_degree: int
    out_degree: int
    total_received: int
    total_sent: int
    clustering_coefficient: Optional[float] = None


class RiskScoreResponse(BaseModel):
    address: str
    score: float
    risk_factors: Dict[str, float]
    explanation: str


class PathRequest(BaseModel):
    source: str
    target: str
    max_hops: int = 5


class PathResponse(BaseModel):
    source: str
    target: str
    path: Optional[List[str]]
    hops: Optional[int]


import asyncio
import logging
import time

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("api")

last_graph_update = None


def rebuild_graph():
    """Rebuild the transaction graph from database"""
    global transaction_graph, risk_analyzer, last_graph_update

    log.info("Rebuilding transaction graph...")

    try:
        # Create fresh graph
        new_graph = TransactionGraph()

        conn = get_db_connection()
        cursor = conn.cursor()

        query = """
        SELECT
            t.tx_hash,
            to_in.address AS input_address,
            to_in.value_satoshis AS input_value,
            to_out.address AS output_address,
            to_out.value_satoshis AS output_value,
            obs.first_seen_at
        FROM transactions t
        JOIN transaction_inputs ti ON t.tx_hash = ti.tx_hash
        JOIN transaction_outputs to_in
            ON ti.prev_tx_hash = to_in.tx_hash
           AND ti.prev_output_idx = to_in.output_index
        JOIN transaction_outputs to_out ON t.tx_hash = to_out.tx_hash
        LEFT JOIN transaction_observations obs ON t.tx_hash = obs.tx_hash
        WHERE to_in.address IS NOT NULL
          AND to_out.address IS NOT NULL
        ORDER BY obs.first_seen_at DESC
        LIMIT 10000
        """

        cursor.execute(query)
        rows = cursor.fetchall()

        tx_data = {}
        for row in rows:
            tx_hash = bytes(row['tx_hash']).hex()
            if tx_hash not in tx_data:
                tx_data[tx_hash] = {
                    'inputs': set(),
                    'outputs': set(),
                    'timestamp': row['first_seen_at']
                }

            if row['input_address']:
                tx_data[tx_hash]['inputs'].add((
                    row['input_address'],
                    int(row['input_value'])
                ))

            if row['output_address']:
                tx_data[tx_hash]['outputs'].add((
                    row['output_address'],
                    int(row['output_value'])
                ))

        for tx_hash, data in tx_data.items():
            if data['inputs'] and data['outputs']:
                new_graph.add_transaction(
                    tx_hash,
                    list(data['inputs']),
                    list(data['outputs']),
                    data['timestamp']
                )

        cursor.close()
        conn.close()

        # Swap in the new graph
        transaction_graph = new_graph
        risk_analyzer = RiskAnalyzer(transaction_graph)
        last_graph_update = datetime.now()

        log.info(f"Graph rebuilt: {transaction_graph.graph.number_of_nodes()} nodes, {transaction_graph.graph.number_of_edges()} edges")

    except Exception as e:
        log.error(f"Failed to rebuild graph: {e}")


async def graph_rebuild_task():
    """Background task to rebuild graph every 2 minutes"""
    while True:
        await asyncio.sleep(120)  # 2 minutes
        rebuild_graph()


@app.on_event("startup")
async def startup_event():
    """Load transaction data with retry, then start background rebuild."""
    max_retries = 10
    for attempt in range(1, max_retries + 1):
        try:
            rebuild_graph()
            break
        except Exception as e:
            if attempt == max_retries:
                log.error(f"Failed to connect after {max_retries} attempts, starting with empty graph")
            else:
                wait = min(attempt * 2, 10)
                log.warning(f"Startup attempt {attempt}/{max_retries} failed: {e}. Retrying in {wait}s...")
                time.sleep(wait)
    asyncio.create_task(graph_rebuild_task())


@app.get("/health")
async def health():
    """Health check endpoint for load balancers and monitoring"""
    return {"status": "healthy", "graph_loaded": last_graph_update is not None}


@app.get("/")
async def root():
    """API health check"""
    return {
        "status": "online",
        "nodes": transaction_graph.graph.number_of_nodes(),
        "edges": transaction_graph.graph.number_of_edges(),
        "version": "0.1.0",
        "last_update": last_graph_update.isoformat() if last_graph_update else None
    }


@app.get("/address/{address}/metrics", response_model=AddressMetricsResponse)
async def get_address_metrics(address: str):
    """Get network metrics for a specific address"""
    metrics = transaction_graph.get_address_metrics(address)
    
    if "error" in metrics:
        raise HTTPException(status_code=404, detail=metrics["error"])
    
    return metrics


@app.get("/address/{address}/risk", response_model=RiskScoreResponse)
async def get_risk_score(address: str):
    """Calculate risk score for an address"""
    if not risk_analyzer:
        raise HTTPException(status_code=503, detail="Risk analyzer not initialized")
    
    risk = risk_analyzer.calculate_risk_score(address)
    
    return {
        "address": risk.address,
        "score": risk.score,
        "risk_factors": risk.risk_factors,
        "explanation": risk.explanation
    }


@app.post("/path", response_model=PathResponse)
async def find_path(request: PathRequest):
    """Find shortest path between two addresses"""
    path = transaction_graph.trace_funds(
        request.source, 
        request.target, 
        request.max_hops
    )
    
    return {
        "source": request.source,
        "target": request.target,
        "path": path,
        "hops": len(path) - 1 if path else None
    }


@app.get("/pagerank")
async def get_pagerank(top_n: int = 10):
    """Get top addresses by PageRank"""
    pagerank = transaction_graph.calculate_pagerank()
    
    # Sort and return top N
    sorted_ranks = sorted(pagerank.items(), key=lambda x: x[1], reverse=True)[:top_n]
    
    return {
        "top_addresses": [
            {"address": addr, "pagerank": score}
            for addr, score in sorted_ranks
        ]
    }


@app.get("/communities")
async def get_communities():
    """Identify transaction communities"""
    communities = transaction_graph.find_communities()
    
    return {
        "total_communities": len(communities),
        "communities": [
            {
                "id": i,
                "size": len(community),
                "addresses": list(community)[:10]  # Return first 10 addresses
            }
            for i, community in enumerate(communities)
        ]
    }


@app.get("/stats")
async def get_stats():
    """Get overall graph statistics"""
    import networkx as nx

    graph = transaction_graph.graph

    stats = {
        "nodes": graph.number_of_nodes(),
        "edges": graph.number_of_edges(),
        "density": nx.density(graph) if graph.number_of_nodes() > 0 else 0,
    }

    if graph.number_of_nodes() > 0:
        # Calculate additional stats (can be expensive)
        try:
            stats["avg_degree"] = sum(dict(graph.degree()).values()) / graph.number_of_nodes()
        except:
            pass

    return stats


@app.get("/country-rankings")
async def get_country_rankings():
    """Get countries ranked by first-seen transaction observations"""
    try:
        conn = get_db_connection()
        cursor = conn.cursor()

        cursor.execute("""
            SELECT
                pc.country_code,
                pc.region,
                COUNT(DISTINCT obs.tx_hash) as first_seen_count,
                COUNT(DISTINCT pc.peer_addr) as peer_count
            FROM peer_connections pc
            JOIN transaction_observations obs ON pc.peer_addr = obs.first_peer_addr
            WHERE pc.country_code IS NOT NULL
            GROUP BY pc.country_code, pc.region
            ORDER BY first_seen_count DESC
            LIMIT 20
        """)

        rows = cursor.fetchall()
        cursor.close()
        conn.close()

        return {
            "rankings": [
                {
                    "country_code": row["country_code"],
                    "region": row["region"],
                    "first_seen_count": row["first_seen_count"],
                    "peer_count": row["peer_count"]
                }
                for row in rows
            ]
        }
    except Exception as e:
        return {"rankings": [], "error": str(e)}


@app.get("/high-risk-addresses")
async def get_high_risk_addresses(top_n: int = 10):
    """Get addresses with highest risk scores"""
    if not risk_analyzer:
        raise HTTPException(status_code=503, detail="Risk analyzer not initialized")

    pagerank = transaction_graph.calculate_pagerank()

    # Build a broad candidate pool from multiple signals
    candidates = set()

    # 1. Top PageRank addresses (high centrality)
    sorted_by_pr = sorted(pagerank.items(), key=lambda x: x[1], reverse=True)[:50]
    candidates.update(addr for addr, _ in sorted_by_pr)

    # 2. Addresses involved in double-spend attempts
    try:
        conn = get_db_connection()
        cur = conn.cursor()
        cur.execute("""
            SELECT DISTINCT address FROM (
                SELECT tout.address
                FROM transaction_observations obs
                JOIN transaction_outputs tout ON obs.tx_hash = tout.tx_hash
                WHERE obs.double_spend_flag = TRUE AND tout.address IS NOT NULL
                UNION
                SELECT prev_out.address
                FROM transaction_observations obs
                JOIN transaction_inputs tin ON obs.tx_hash = tin.tx_hash
                JOIN transaction_outputs prev_out
                    ON tin.prev_tx_hash = prev_out.tx_hash
                    AND tin.prev_output_idx = prev_out.output_index
                WHERE obs.double_spend_flag = TRUE AND prev_out.address IS NOT NULL
            ) ds_addrs
        """)
        candidates.update(row["address"] for row in cur.fetchall())
        cur.close()
        conn.close()
    except Exception:
        pass

    # 3. Addresses with high in+out degree (potential mixers)
    for node in transaction_graph.graph.nodes():
        in_deg = transaction_graph.graph.in_degree(node)
        out_deg = transaction_graph.graph.out_degree(node)
        if in_deg > 50 and out_deg > 50:
            candidates.add(node)

    risks = []
    for addr in candidates:
        try:
            risk = risk_analyzer.calculate_risk_score(addr)
            risks.append({
                "address": addr,
                "risk_score": risk.score,
                "pagerank": pagerank.get(addr, 0),
                "factors": risk.risk_factors,
                "explanation": risk.explanation
            })
        except:
            pass

    # Sort by risk score and return top N
    risks.sort(key=lambda x: x["risk_score"], reverse=True)
    return {"high_risk_addresses": risks[:top_n]}


@app.get("/propagation-stats")
async def get_propagation_stats():
    """Get transaction propagation statistics by region"""
    try:
        conn = get_db_connection()
        cursor = conn.cursor()

        cursor.execute("""
            SELECT
                pc.region,
                COUNT(*) as observation_count,
                AVG(pe.delay_from_first_ms) as avg_delay_ms,
                MIN(pe.delay_from_first_ms) as min_delay_ms,
                MAX(pe.delay_from_first_ms) as max_delay_ms
            FROM propagation_events pe
            JOIN peer_connections pc ON pe.peer_addr = pc.peer_addr
            WHERE pc.region IS NOT NULL
            GROUP BY pc.region
            ORDER BY observation_count DESC
        """)

        rows = cursor.fetchall()
        cursor.close()
        conn.close()

        return {
            "by_region": [
                {
                    "region": row["region"],
                    "observation_count": row["observation_count"],
                    "avg_delay_ms": float(row["avg_delay_ms"]) if row["avg_delay_ms"] else 0,
                    "min_delay_ms": row["min_delay_ms"],
                    "max_delay_ms": row["max_delay_ms"]
                }
                for row in rows
            ]
        }
    except Exception as e:
        return {"by_region": [], "error": str(e)}


@app.get("/geo-activity")
async def get_geo_activity():
    """Get recent transaction activity by geographic location for world map"""
    try:
        conn = get_db_connection()
        cursor = conn.cursor()

        # Get transaction counts by country in the last hour
        cursor.execute("""
            SELECT
                pc.country_code,
                pc.latitude,
                pc.longitude,
                COUNT(DISTINCT obs.tx_hash) as tx_count
            FROM transaction_observations obs
            JOIN peer_connections pc ON obs.first_peer_addr = pc.peer_addr
            WHERE pc.country_code IS NOT NULL
              AND pc.latitude IS NOT NULL
              AND pc.longitude IS NOT NULL
              AND obs.first_seen_at > NOW() - INTERVAL '1 hour'
            GROUP BY pc.country_code, pc.latitude, pc.longitude
            ORDER BY tx_count DESC
        """)

        rows = cursor.fetchall()
        cursor.close()
        conn.close()

        return {
            "locations": [
                {
                    "country_code": row["country_code"],
                    "lat": float(row["latitude"]),
                    "lng": float(row["longitude"]),
                    "tx_count": row["tx_count"]
                }
                for row in rows
            ]
        }
    except Exception as e:
        return {"locations": [], "error": str(e)}


@app.get("/peer-locations")
async def get_peer_locations():
    """Get geographic locations of all peers for network visualization"""
    try:
        conn = get_db_connection()
        cursor = conn.cursor()

        # Get unique peer locations (group by location to avoid duplicates)
        cursor.execute("""
            SELECT
                pc.country_code,
                pc.latitude,
                pc.longitude,
                pc.city,
                COUNT(*) as peer_count,
                SUM(CASE WHEN pc.disconnected_at IS NULL THEN 1 ELSE 0 END) as active_count
            FROM peer_connections pc
            WHERE pc.latitude IS NOT NULL
              AND pc.longitude IS NOT NULL
            GROUP BY pc.country_code, pc.latitude, pc.longitude, pc.city
        """)

        rows = cursor.fetchall()
        cursor.close()
        conn.close()

        return {
            "peers": [
                {
                    "country_code": row["country_code"],
                    "lat": float(row["latitude"]),
                    "lng": float(row["longitude"]),
                    "city": row["city"],
                    "peer_count": row["peer_count"],
                    "active": row["active_count"] > 0
                }
                for row in rows
            ]
        }
    except Exception as e:
        return {"peers": [], "error": str(e)}


@app.get("/observer-location")
async def get_observer_location():
    """Get the observer's location based on public IP address"""
    import urllib.request
    import json

    try:
        # Get public IP and location using ip-api.com (free, no key needed)
        with urllib.request.urlopen('http://ip-api.com/json/', timeout=5) as response:
            data = json.loads(response.read().decode())
            if data.get('status') == 'success':
                return {
                    "lat": data.get('lat'),
                    "lng": data.get('lon'),
                    "city": data.get('city'),
                    "country": data.get('country'),
                    "ip": data.get('query')
                }
    except Exception as e:
        pass

    # Fallback to a default location if lookup fails
    return {"lat": 51.5074, "lng": -0.1278, "city": "Unknown", "country": "Unknown", "ip": "Unknown"}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
