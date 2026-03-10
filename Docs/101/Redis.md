## Future Redis usage

Redis is intentionally included in `infra/k8s` but not yet wired into the code, to leave space for future exercises:

- Caching frequently requested job results to reduce DB load.
- Implementing a **read‑through** cache for `GET /jobs/:id`.
- Storing short‑lived rate‑limit counters or deduplication keys.

When you introduce Redis into the code, you will be able to compare:

- **Pure DB reads** vs. **DB + cache**, and
- Observe the difference in metrics and latency in Grafana and Jaeger.
