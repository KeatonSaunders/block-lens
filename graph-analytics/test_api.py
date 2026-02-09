"""
Quick smoke tests for the Bitcoin Graph Analytics API.

Usage:
    Start the server first:  python api.py
    Then run:                 python test_api.py
"""

import requests
import sys

BASE = "http://localhost:8000"


def test(name, method, path, **kwargs):
    url = f"{BASE}{path}"
    print(f"\n--- {name} ---")
    print(f"{method} {url}")
    resp = getattr(requests, method.lower())(url, **kwargs)
    print(f"Status: {resp.status_code}")
    try:
        print(resp.json())
    except:
        print(resp.text)
    return resp


def main():
    # Health check
    r = test("Health check", "GET", "/")
    assert r.status_code == 200

    # Health endpoint
    r = test("Health endpoint", "GET", "/health")
    assert r.status_code == 200

    # Graph stats
    r = test("Graph stats", "GET", "/stats")
    assert r.status_code == 200

    # PageRank top 5
    r = test("PageRank top 5", "GET", "/pagerank?top_n=5")
    assert r.status_code == 200
    top = r.json().get("top_addresses", [])

    # Communities
    r = test("Communities", "GET", "/communities")
    assert r.status_code == 200

    # If we got addresses from pagerank, test address-specific endpoints
    if top:
        addr = top[0]["address"]
        test("Address metrics", "GET", f"/address/{addr}/metrics")
        test("Address risk", "GET", f"/address/{addr}/risk")

    # If we got at least 2 addresses, test path tracing
    if len(top) >= 2:
        test(
            "Trace funds",
            "POST",
            "/path",
            json={"source": top[0]["address"], "target": top[1]["address"]},
        )

    # Analytics endpoints
    test("Country rankings", "GET", "/country-rankings")
    test("Propagation stats", "GET", "/propagation-stats")
    test("High risk addresses", "GET", "/high-risk-addresses?top_n=5")
    test("Geo activity", "GET", "/geo-activity")
    test("Peer locations", "GET", "/peer-locations")

    print("\n=== All tests passed ===")


if __name__ == "__main__":
    try:
        main()
    except requests.ConnectionError:
        print(f"Could not connect to {BASE}. Is the server running?", file=sys.stderr)
        sys.exit(1)
