"""
Bitcoin Transaction Graph Analytics

Analyzes transaction networks to identify risk patterns and trace fund flows.
"""

import json
import os

import networkx as nx
import psycopg2
from psycopg2.extras import RealDictCursor
from typing import Dict, List, Tuple, Optional
from dataclasses import dataclass
from datetime import datetime
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

CONFIG_PATH = os.path.join(
    os.path.dirname(__file__), '..', 'btc-observer', 'cmd', 'observer', 'config.json'
)


def load_config(path=CONFIG_PATH):
    """Load DB config from environment variables, falling back to config file."""
    # Prefer environment variables (set by docker-compose)
    if os.getenv("DB_HOST"):
        return {
            "db_host": os.getenv("DB_HOST", "localhost"),
            "db_port": int(os.getenv("DB_PORT", "5432")),
            "db_user": os.getenv("DB_USER", "postgres"),
            "db_password": os.getenv("DB_PASSWORD", ""),
            "db_name": os.getenv("DB_NAME", "bitcoin_intel"),
        }

    # Fall back to config file for local development
    with open(path) as f:
        return json.load(f)


def get_db_connection(cfg=None):
    if cfg is None:
        cfg = load_config()
    return psycopg2.connect(
        host=cfg["db_host"],
        port=cfg["db_port"],
        user=cfg["db_user"],
        password=cfg["db_password"],
        database=cfg["db_name"],
        cursor_factory=RealDictCursor,
    )


@dataclass
class RiskScore:
    """Risk assessment for a Bitcoin address"""
    address: str
    score: float  # 0-100
    risk_factors: Dict[str, float]
    explanation: str


class TransactionGraph:
    """
    Builds and analyzes Bitcoin transaction networks.
    
    Nodes: Bitcoin addresses
    Edges: Transactions (weighted by value)
    """
    
    def __init__(self):
        self.graph = nx.DiGraph()
        self.address_metadata = {}
        self.transaction_timing = {}
        self._pagerank_cache = None
        
    def add_transaction(
        self, 
        tx_hash: str, 
        inputs: List[Tuple[str, int]], 
        outputs: List[Tuple[str, int]],
        timestamp: Optional[datetime] = None
    ):
        """
        Add a transaction to the graph.
        
        Args:
            tx_hash: Transaction hash
            inputs: List of (address, value) tuples
            outputs: List of (address, value) tuples
            timestamp: When transaction was first seen
        """
        if timestamp:
            self.transaction_timing[tx_hash] = timestamp
            
        # Create edges from inputs to outputs
        for input_addr, input_value in inputs:
            for output_addr, output_value in outputs:
                # Calculate proportion of value flowing
                weight = output_value / sum(v for _, v in outputs)
                
                if self.graph.has_edge(input_addr, output_addr):
                    # Update existing edge
                    self.graph[input_addr][output_addr]['weight'] += weight
                    self.graph[input_addr][output_addr]['tx_count'] += 1
                    self.graph[input_addr][output_addr]['total_value'] += output_value
                else:
                    # Add new edge
                    self.graph.add_edge(
                        input_addr, 
                        output_addr, 
                        weight=weight,
                        tx_count=1,
                        total_value=output_value,
                        first_tx=tx_hash
                    )
                    
        logger.info(f"Added transaction {tx_hash[:8]} to graph")
    
    def calculate_pagerank(self, alpha=0.85) -> Dict[str, float]:
        """
        Calculate PageRank for all addresses in the graph.
        Higher score = more central/influential address.
        Results are cached and reused until the graph is rebuilt.
        """
        if len(self.graph) == 0:
            return {}

        if self._pagerank_cache is not None:
            return self._pagerank_cache

        pagerank = nx.pagerank(self.graph, alpha=alpha, weight='weight')
        self._pagerank_cache = pagerank
        logger.info(f"Calculated PageRank for {len(pagerank)} addresses")
        return pagerank
    
    def find_communities(self) -> List[set]:
        """
        Identify clusters of addresses that transact together.
        Uses Louvain community detection algorithm.
        """
        if len(self.graph) == 0:
            return []
            
        # Convert to undirected for community detection
        undirected = self.graph.to_undirected()
        
        # Use Louvain algorithm
        communities = nx.community.louvain_communities(undirected, weight='weight')
        
        logger.info(f"Found {len(communities)} communities")
        return communities
    
    def trace_funds(
        self, 
        source: str, 
        target: str, 
        max_hops: int = 5
    ) -> Optional[List[str]]:
        """
        Find shortest path from source to target address.
        Returns list of addresses in the path, or None if no path exists.
        """
        try:
            path = nx.shortest_path(self.graph, source, target)
            logger.info(f"Found path from {source[:8]} to {target[:8]}: {len(path)} hops")
            return path
        except nx.NetworkXNoPath:
            logger.info(f"No path found from {source[:8]} to {target[:8]}")
            return None
        except nx.NodeNotFound as e:
            logger.warning(f"Address not found in graph: {e}")
            return None
    
    def get_address_metrics(self, address: str) -> Dict:
        """
        Get network metrics for a specific address.
        """
        if address not in self.graph:
            return {"error": "Address not found in graph"}
        
        metrics = {
            "address": address,
            "in_degree": self.graph.in_degree(address),
            "out_degree": self.graph.out_degree(address),
            "total_received": sum(
                data['total_value'] 
                for _, _, data in self.graph.in_edges(address, data=True)
            ),
            "total_sent": sum(
                data['total_value'] 
                for _, _, data in self.graph.out_edges(address, data=True)
            ),
        }
        
        # Calculate clustering coefficient (how connected neighbors are)
        undirected = self.graph.to_undirected()
        if address in undirected:
            metrics["clustering_coefficient"] = nx.clustering(undirected, address)
        
        return metrics


class RiskAnalyzer:
    """
    Analyzes transaction patterns to assign risk scores.
    """

    def __init__(self, graph: TransactionGraph):
        self.graph = graph
        self.known_risks = {}  # Could load from database

    def _check_double_spend_involvement(self, address: str) -> dict:
        """
        Check if address is involved in any double-spend attempts.
        Returns dict with count and details.
        """
        try:
            conn = get_db_connection()
            cur = conn.cursor()
            # Find transactions involving this address that have double_spend_flag
            cur.execute("""
                SELECT DISTINCT encode(obs.tx_hash, 'hex') as tx_hash
                FROM transaction_observations obs
                JOIN transaction_outputs tout ON obs.tx_hash = tout.tx_hash
                WHERE obs.double_spend_flag = TRUE
                  AND tout.address = %s
                UNION
                SELECT DISTINCT encode(obs.tx_hash, 'hex') as tx_hash
                FROM transaction_observations obs
                JOIN transaction_inputs tin ON obs.tx_hash = tin.tx_hash
                JOIN transaction_outputs prev_out
                    ON tin.prev_tx_hash = prev_out.tx_hash
                    AND tin.prev_output_idx = prev_out.output_index
                WHERE obs.double_spend_flag = TRUE
                  AND prev_out.address = %s
            """, (address, address))

            rows = cur.fetchall()
            cur.close()
            conn.close()

            return {
                "count": len(rows),
                "tx_hashes": [row["tx_hash"] for row in rows]
            }
        except Exception as e:
            logger.warning(f"Error checking double-spend for {address}: {e}")
            return {"count": 0, "tx_hashes": []}

    def calculate_risk_score(self, address: str, pagerank: Optional[Dict[str, float]] = None) -> RiskScore:
        """
        Calculate comprehensive risk score for an address.
        
        Risk factors:
        - Connection to known bad actors (if any flagged)
        - Unusual transaction patterns
        - Network centrality
        - Mixing service indicators
        """
        if address not in self.graph.graph:
            return RiskScore(
                address=address,
                score=0.0,
                risk_factors={},
                explanation="Address not found in transaction graph"
            )
        
        risk_factors = {}

        # Risk factor weights (sum to 100):
        # - double_spend: 45 (direct evidence of malicious behavior)
        # - potential_mixer: 25 (strong structural indicator)
        # - high_centrality: 15 (hub activity)
        # - high_volume: 10 (unusual transaction count)
        # - low_clustering: 5 (isolated patterns)

        # 1. Check for double-spend involvement (strongest signal - 45 pts)
        ds_info = self._check_double_spend_involvement(address)
        if ds_info["count"] > 0:
            risk_factors["double_spend"] = 45

        # 2. Check for mixing patterns (many inputs AND outputs - 25 pts)
        metrics = self.graph.get_address_metrics(address)
        if metrics["in_degree"] > 50 and metrics["out_degree"] > 50:
            risk_factors["potential_mixer"] = 25

        # 3. Check network centrality (high PageRank - 15 pts)
        if pagerank is None:
            pagerank = self.graph.calculate_pagerank()
        pr_score = pagerank.get(address, 0)
        if pr_score > 0.01:
            risk_factors["high_centrality"] = min(pr_score * 1500, 15)

        # 4. Check transaction velocity (high volume - 10 pts)
        total_tx = metrics["in_degree"] + metrics["out_degree"]
        if total_tx > 100:
            risk_factors["high_volume"] = min(total_tx / 100, 10)

        # 5. Check clustering coefficient (low clustering - 5 pts)
        if "clustering_coefficient" in metrics:
            if metrics["clustering_coefficient"] < 0.1:
                risk_factors["low_clustering"] = 5

        # Calculate final score (weights sum to 100, no cap needed)
        total_score = sum(risk_factors.values())
        
        # Generate explanation
        explanation = self._generate_explanation(risk_factors)
        
        return RiskScore(
            address=address,
            score=total_score,
            risk_factors=risk_factors,
            explanation=explanation
        )
    
    def _generate_explanation(self, risk_factors: Dict[str, float]) -> str:
        """Generate human-readable explanation of risk factors"""
        if not risk_factors:
            return "No significant risk factors detected"

        explanations = []
        if "double_spend" in risk_factors:
            explanations.append("INVOLVED IN DOUBLE-SPEND ATTEMPT (direct evidence of malicious activity)")
        if "high_centrality" in risk_factors:
            explanations.append("High network centrality (influential address)")
        if "high_volume" in risk_factors:
            explanations.append("High transaction volume")
        if "potential_mixer" in risk_factors:
            explanations.append("Transaction pattern consistent with mixing service")
        if "low_clustering" in risk_factors:
            explanations.append("Low clustering coefficient (limited peer connections)")

        return "; ".join(explanations)


def main():
    """Load real transactions from the database and run analytics."""
    logger.info("Connecting to database...")
    conn = get_db_connection()
    cur = conn.cursor()

    logger.info("Querying transactions...")
    cur.execute("""
        SELECT
            t.tx_hash,
            to_in.address   AS input_address,
            to_in.value_satoshis AS input_value,
            to_out.address  AS output_address,
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
        LIMIT 50000
    """)
    rows = cur.fetchall()
    cur.close()
    conn.close()

    # Group rows by transaction
    tx_data = {}
    for row in rows:
        tx_hash = bytes(row["tx_hash"]).hex()
        if tx_hash not in tx_data:
            tx_data[tx_hash] = {
                "inputs": set(),
                "outputs": set(),
                "timestamp": row["first_seen_at"],
            }
        if row["input_address"]:
            tx_data[tx_hash]["inputs"].add(
                (row["input_address"], int(row["input_value"]))
            )
        if row["output_address"]:
            tx_data[tx_hash]["outputs"].add(
                (row["output_address"], int(row["output_value"]))
            )

    logger.info(f"Loaded {len(tx_data)} transactions from database")

    # Build graph
    graph = TransactionGraph()
    for tx_hash, data in tx_data.items():
        if data["inputs"] and data["outputs"]:
            graph.add_transaction(
                tx_hash,
                list(data["inputs"]),
                list(data["outputs"]),
                data["timestamp"],
            )

    logger.info(
        f"Graph has {graph.graph.number_of_nodes()} nodes "
        f"and {graph.graph.number_of_edges()} edges"
    )

    # PageRank
    pagerank = graph.calculate_pagerank()
    top = sorted(pagerank.items(), key=lambda x: x[1], reverse=True)[:10]
    logger.info("Top 10 addresses by PageRank:")
    for addr, score in top:
        logger.info(f"  {addr}  {score:.6f}")

    # Communities
    communities = graph.find_communities()
    logger.info(f"Found {len(communities)} communities")

    # Risk analysis on the top-ranked address
    if top:
        analyzer = RiskAnalyzer(graph)
        risk = analyzer.calculate_risk_score(top[0][0])
        logger.info(f"Risk score for top address: {risk.score:.2f}")
        logger.info(f"Risk explanation: {risk.explanation}")


if __name__ == "__main__":
    main()
