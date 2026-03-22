# ClawMatrix — Monitoring Stack

VictoriaMetrics + Grafana for ClawMatrix fleet metrics.

## Start

```bash
cd examples/monitoring
docker compose up -d
```

- **Grafana**: http://localhost:3000 (admin / admin)
- **VictoriaMetrics**: http://localhost:8428

The stack scrapes the control plane at `host.docker.internal:8080/metrics` every 15s. The **ClawMatrix Fleet** dashboard loads automatically.

## Dashboard panels

| Panel | Description |
|-------|-------------|
| Healthy Agents | Count of agents with `agent_health == 1` |
| Total Agents | All registered agents |
| Requests Allowed/Blocked (rate) | Per-second egress traffic |
| Agent Health table | Per-agent health, request counts, latency |
| Traffic timeseries | Allowed vs blocked over time, per agent |
| Avg Latency | Per-agent latency trend |
| Cumulative Requests | Total requests per agent |
| Go Runtime | Control plane heap and goroutine count |

Use the **Registration** and **Agent** dropdowns to filter to a specific subset.

## Pointing at a remote control plane

Edit `docker-compose.yml` and change the scrape target:

```yaml
configs:
  vm_scrape:
    content: |
      scrape_configs:
        - job_name: clawmatrix
          static_configs:
            - targets:
                - your-cp-host:8080
```
