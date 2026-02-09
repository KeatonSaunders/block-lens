# Risk Model: Network-Based Transaction Analysis

This document describes the risk scoring methodology implemented in this Bitcoin Network Intelligence Platform, along with the theoretical foundations and potential enhancements informed by academic research.

## Table of Contents
- [Current Implementation](#current-implementation)
- [Theoretical Foundations](#theoretical-foundations)
- [Future Enhancements](#future-enhancements)
- [Limitations](#limitations)
- [References](#references)

---

## Current Implementation

The risk scoring model evaluates Bitcoin addresses based on their structural position in the transaction graph. The implementation uses NetworkX for graph analysis.

### Risk Factors

Weights are normalized to sum to 100, representing relative importance:

| Factor | Weight | Trigger Condition | Rationale |
|--------|--------|-------------------|-----------|
| **Double-Spend** | 45 | Address involved in flagged double-spend tx | Direct evidence of malicious behavior—attempting to spend the same UTXO twice |
| **Potential Mixer** | 25 | In-degree > 50 AND out-degree > 50 | Many-to-many transaction patterns are consistent with mixing/tumbling services |
| **High Centrality** | 15 | PageRank > 0.01 | Addresses with high PageRank receive funds from many influential sources—characteristic of exchanges, mixers, or aggregation services |
| **High Volume** | 10 | Total degree > 100 | Addresses with many counterparties exhibit non-typical wallet behavior |
| **Low Clustering** | 5 | Clustering coefficient < 0.1 | Isolated addresses not connected to peer clusters may indicate deliberate obfuscation |
| **Total** | **100** | | |

### Score Calculation

```python
total_score = sum(risk_factors.values())  # Weights sum to 100
```

Since weights are normalized, an address can only reach 100 if it triggers all risk factors.

### Score Interpretation

| Score | Risk Level | Interpretation |
|-------|------------|----------------|
| 0-25 | Low | Normal wallet behavior |
| 25-50 | Medium | Elevated activity, warrants monitoring |
| 50-75 | High | Patterns consistent with mixing or high-volume services |
| 75-100 | Critical | Strong indicators of mixing/laundering patterns |

### What the Current Model Does NOT Use

The database captures several signals that are **not yet incorporated** into risk scoring:

1. **Propagation timing** (`first_seen_at`, `first_seen_peer` in observations)
2. **Geographic origin** (which country's peer first announced the transaction)
3. **Temporal patterns** (transaction timing, intervals, bursts)
4. **Block confirmation data** (`confirmed_in_block`, `confirmations`)

These represent opportunities for enhancement (see [Future Enhancements](#future-enhancements)).

---

## Theoretical Foundations

### Graph-Based Risk Analysis

The core approach—treating the transaction network as a directed graph and analyzing structural properties—is well-established in blockchain forensics research.

**PageRank for Influence Detection**

PageRank, originally developed for web page ranking, translates naturally to transaction networks: an address has high PageRank if it receives funds from other high-PageRank addresses. Research has shown this effectively identifies influential entities like exchanges and large services (Nerurkar et al., 2022).

**Community Detection**

The Louvain algorithm identifies clusters of addresses that transact frequently together. In Bitcoin, these communities often correspond to:
- Wallet clusters (addresses controlled by the same entity)
- Service ecosystems (exchange hot wallets, merchant processors)
- Mixing pools

**Clustering Coefficient**

Low clustering coefficient indicates an address whose counterparties don't transact with each other—a pattern seen in:
- Privacy-focused users deliberately avoiding address reuse
- Mixing services designed to break transaction graph links
- One-time payment addresses

### Network Propagation Analysis (Background)

Bitcoin transactions propagate through the P2P network before confirmation. Academic research has modeled this as a non-homogeneous Poisson process, with blocks reaching 90% of nodes within ~10 seconds (Decker & Wattenhofer, 2013).

The peer that first announces a transaction is likely topologically closer to the broadcast origin. By observing from geographically diverse nodes, we can statistically infer broadcast regions—valuable for compliance and forensics.

**This platform captures propagation data but does not yet incorporate it into risk scoring.**

### Machine Learning Approaches (Literature Review)

Recent research has demonstrated several ML approaches for blockchain anomaly detection:

- **Graph Neural Networks**: GCN and GAT models have shown strong performance in AML/CFT applications, outperforming classical approaches (Alarab et al., 2023)
- **Explainable AI**: SHAP values effectively identify which features drive transaction classification (Alarab et al., 2024)
- **Unsupervised Methods**: Isolation Forest and One-Class SVM detect anomalies without labeled training data (Pham & Lee, 2016)
- **Hybrid Approaches**: K-Means clustering combined with Z-score analysis for anomaly detection (Zola et al., 2025)

---

## Future Enhancements

The following enhancements would leverage data already captured by the observer:

### 1. Propagation Anomaly Score

**Data available:** `first_seen_at`, `first_seen_peer`, peer geolocation

**Proposed enhancement:**
- Calculate propagation velocity (regions reached per second)
- Compare against baseline for similar transactions
- Flag statistical outliers (Z-score > 2σ)

```python
propagation_zscore = calculate_propagation_zscore(tx_hash)
if abs(propagation_zscore) > 2:
    risk_factors["propagation_anomaly"] = min(abs(propagation_zscore) * 10, 25)
```

### 2. Geographic Origin Analysis

**Data available:** `first_seen_peer` → peer country

**Proposed enhancement:**
- Track which regions typically announce an address's transactions first
- Flag inconsistent geographic patterns (transactions from same wallet appearing in different regions)

### 3. Temporal Pattern Detection

**Data available:** `first_seen_at` timestamps

**Proposed enhancement:**
- Detect regular-interval transactions (bot/automated behavior)
- Identify burst patterns (stress testing, coordinated activity)
- Calculate transaction timing entropy

```python
timing_entropy = calculate_timing_entropy(address_transactions)
if timing_entropy < threshold:  # Low entropy = regular patterns
    risk_factors["automated_behavior"] = 20
```

### 4. Confirmation Timing Analysis

**Data available:** `confirmed_in_block`, `confirmations`, block timestamps

**Proposed enhancement:**
- Track time-to-confirmation distribution
- Flag transactions with unusual confirmation patterns
- Identify potential fee manipulation

---

## Practical Assessment: Who Does This Catch?

An honest evaluation of what this risk model can and cannot identify.

### What It Can Catch

| Actor | Why It Works |
|-------|--------------|
| **Double-spend attackers** | Direct evidence—attempting to spend the same UTXO twice is unambiguously malicious |
| **Mixing services** | Many-to-many transaction patterns with low clustering are structurally distinctive |
| **Large exchanges/services** | High PageRank, high volume, many counterparties (though not inherently "risky") |

### What It Cannot Distinguish

The model flags "big and busy" addresses, but cannot differentiate between:

- **Coinbase** (legitimate exchange) vs **mixing service** (potential money laundering)
- Both have: high PageRank, high volume, many-to-many patterns, low clustering

Without external context, the model produces **false positives on legitimate high-volume services**.

### What It Misses Entirely

| Actor | Why It's Missed |
|-------|-----------------|
| **One-time scammer** | Receives ransom, cashes out immediately—no unusual graph structure |
| **Stolen fund recipient** | Unless they're a hub, receiving stolen funds doesn't change graph metrics |
| **Sophisticated actors** | Can deliberately mimic normal wallet patterns |
| **Low-volume bad actors** | Don't trigger volume-based thresholds |

### What Would Make It Useful

For production-grade risk scoring, you need:

1. **External address labels** — Known exchange addresses, OFAC sanctions lists, darknet market wallets, ransomware addresses. This is the differentiator that compliance companies sell.

2. **Taint analysis** — Trace funds N hops from known bad addresses. An address 2 hops from a ransomware wallet is riskier than one with no connection.

3. **Temporal patterns** — Scammers often exhibit distinctive timing: immediate cash-out after receiving funds, regular automated withdrawals, activity correlated with specific events.

4. **Cross-chain analysis** — Track funds moving through bridges to other blockchains, which is increasingly common for laundering.

5. **Clustering/Entity resolution** — Group addresses controlled by the same entity (common-input-ownership heuristic, change address detection) to assess entity-level risk rather than address-level.

### The Honest Pitch

This platform demonstrates the **methodology and infrastructure** for network-layer blockchain analysis:
- Real-time P2P observation from geographically diverse nodes
- Transaction graph construction and analysis
- Propagation timing capture
- Double-spend detection

The risk scoring shows understanding of graph-based approaches, but production effectiveness requires the proprietary labeled datasets that distinguish legitimate compliance platforms.

---

## Limitations

### Current Implementation Limitations

1. **Graph-only analysis**: Ignores temporal and network-layer signals already in the database
2. **Static thresholds**: Hard-coded values (e.g., PageRank > 0.01) may not generalize
3. **No ground truth**: Scores are heuristic—no labeled data for validation
4. **No address clustering**: Treats each address independently rather than grouping by entity

### Fundamental Limitations

1. **Partial visibility**: We observe a subset of the network, not all nodes
2. **Correlation ≠ causation**: High-risk patterns don't prove malicious intent
3. **Adaptive adversaries**: Sophisticated actors can mimic normal patterns
4. **Privacy vs. risk**: Privacy-preserving behavior (CoinJoin, etc.) triggers risk indicators despite being legitimate

### Ethical Considerations

- Risk scores should inform investigation, not replace it
- False positives can harm legitimate privacy-conscious users
- This tool should be used within appropriate legal frameworks

---

## References

### Peer-Reviewed Papers

Alarab, I., Prakoonwit, S., & Naqi, S. (2023). "Detecting anomalous cryptocurrency transactions: An AML/CFT application of machine learning-based forensics." *Electronic Markets*, Springer. [Link](https://link.springer.com/article/10.1007/s12525-023-00654-3)

Alarab, I., et al. (2024). "Detecting anomalies in blockchain transactions using machine learning classifiers and explainability analysis." *Blockchain: Research and Applications*, Elsevier. [Link](https://www.sciencedirect.com/science/article/pii/S2096720924000204)

Decker, C., & Wattenhofer, R. (2013). "Information propagation in the Bitcoin network." *IEEE P2P 2013 Proceedings*.

Nerurkar, P., et al. (2022). "Bitcoin's Blockchain Data Analytics: A Graph Theoretic Perspective." *Springer*. [Link](https://link.springer.com/chapter/10.1007/978-3-030-99584-3_40)

Pham, T., & Lee, S. (2016). "Anomaly Detection in Bitcoin Network Using Unsupervised Learning Methods." *arXiv preprint*. [Link](https://arxiv.org/pdf/1611.03941)

Weber, M., et al. (2025). "A Temporal Graph Dataset of Bitcoin Entity-Entity Transactions." *Scientific Data*, Nature. [Link](https://www.nature.com/articles/s41597-025-04595-8)

Zola, F., et al. (2025). "Leveraging K-Means Clustering and Z-Score for Anomaly Detection in Bitcoin Transactions." *Informatics*, MDPI. [Link](https://www.mdpi.com/2227-9709/12/2/43)

### Additional Resources

MDPI (2025). "Fraud Detection in Cryptocurrency Networks—An Exploration Using Anomaly Detection and Heterogeneous Graph Transformers." *Future Internet*. [Link](https://www.mdpi.com/1999-5903/17/1/44)

Nature (2025). "Bitcoin research with a transaction graph dataset." *Scientific Data*. [Link](https://www.nature.com/articles/s41597-025-04684-8)

---

*This risk model is provided for educational and demonstration purposes. Production risk scoring would require validation against labeled datasets and should incorporate additional signals.*
