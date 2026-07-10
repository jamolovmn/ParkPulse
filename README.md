# ParkPulse

Real-time monitoring for barrier-gate parking systems. ParkPulse tails the
parking controller's Docker logs, reconstructs each car's ANPR → payment →
barrier-open chain, and surfaces two things operators actually care about:

- **How fast** the system reacts (ANPR-to-payment latency, broken down per stage).
- **Why the barrier opened** — classified into four states, so a genuine
  "ghost opening" is never confused with a normal paid exit or a network hiccup.

It also monitors the LAN devices around the gate (cameras, relays, POS
terminals) with ping quality metrics, watches server/container health, and
exposes everything over WebSocket to a single-page dashboard and over a
Prometheus `/metrics` endpoint.

It ships as **one Docker image** (Go backend + embedded Next.js static UI) and
touches only the log stream — it never connects to the parking database.

<!-- Screenshot: add docs/dashboard.png -->

## Why

On a barrier gate, "the barrier opened" and "a car paid and left" are not the
same event, and conflating them hides real problems. ParkPulse separates every
opening into four states:

| State | Meaning | Counted as ghost? | Logged? |
|-------|---------|-------------------|---------|
| **Paid** | Payment went through in software, then the barrier opened. | No | No |
| **Remote** | Car on the sensor, guard opened with the remote; the system auto-charged on exit. | No | No |
| **Violation** | Car on the sensor with an outstanding debt — opened with no payment and no remote. | **Yes** | Yes (+ log context) |
| **Ghost** | No car on the sensor at all, yet the barrier opened by itself. | **Yes** | Yes (+ log context) |
| **Entry** | Car entered an `enter` gate (no payment expected). | No | No |

Only **Violation** and **Ghost** are real anomalies. They increment the ghost
counter and are saved with the surrounding log lines as evidence; the harmless
states appear in the feed but write no log. Crucially, a `Connection is closed`
line from the relay hardware is treated as noise, **not** an opening — this is
the single most common source of false ghost alerts.

## Features

- **Latency tracing** — ANPR → Gateway → DB → POS chain with a per-stage
  breakdown. Remote/auto-pay openings are flagged and excluded from the average
  so a driver's dwell time never inflates the KPI.
- **Four-state opening classifier** (see above).
- **24-hour traffic chart** — hourly entries vs. exits.
- **Network device monitoring** — subnet scan, device fingerprinting
  (camera/web/unknown + vendor), and per-device **ping quality**: jitter, packet
  loss, uptime %, min/avg/max, and a live RTT sparkline. Works even for devices
  that block ICMP (TCP fallback).
- **Server & container health** — CPU per core, RAM, uptime, and `docker stats`
  per container.
- **Internet speedtest** — periodic download/upload/ping via Cloudflare.
- **Prometheus `/metrics`** — plug straight into Grafana.
- **YAML config** — declarative, git-friendly alternative to env vars.

## Quick start

```bash
docker run -d --name parkpulse \
  -e TARGET_CONTAINER=p24gui \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --network host \
  ghcr.io/jamolovmn/parking-pulse:latest
```

Then open `http://localhost:8888`.

- `TARGET_CONTAINER` — the container whose logs to read (comma-separated for
  several). This is the only required setting.
- The Docker socket mount lets ParkPulse tail logs and read `docker stats`.
- `--network host` is what makes LAN device scanning and pinging work.

### Build it yourself

```bash
./build.sh --local   # build the image locally (no push)
```

The [Dockerfile](Dockerfile) builds the Next.js UI to static files and the Go
binary, then ships a ~29 MB Alpine image.

## Configuration

Every setting is an environment variable, and **explicit env always wins**. A
YAML file is an optional convenience — see [`parkpulse.example.yaml`](parkpulse.example.yaml).
Copy it to `parkpulse.yaml` (working dir), or `/etc/parkpulse/config.yaml`, or
point `CONFIG_FILE` at it.

| Env | Default | Purpose |
|-----|---------|---------|
| `TARGET_CONTAINER` | — (required) | Container name(s) to tail, comma-separated. |
| `LISTEN_ADDR` | `:8888` | HTTP/WebSocket listen address. |
| `DEVICES` | — | Monitored devices: `name=ip,name=ip`. |
| `SCAN_SUBNET` | auto | Subnet(s) for the scanner, e.g. `192.168.1.0/24`. |
| `SPEEDTEST_MIN` | `15` | Speedtest interval in minutes (`0` disables). |
| `MATCH_WINDOW_SEC` | `180` | ANPR→payment correlation window. |
| `AUTOPAY_SEC` | `90` | How long to wait for auto-payment after an opening (remote vs. violation). |
| `PRESENCE_SEC` | `60` | How long an ANPR read counts as "car on the sensor". |
| `RELAY_OPEN_RE` | built-in | Regex for the physical barrier-open log line. |
| `RELAY_REMOTE_RE` | built-in | Regex for the guard's remote-open signal. |

The two regexes let you adapt ParkPulse to a controller whose log wording
differs, without rebuilding.

## Grafana / Prometheus

ParkPulse exposes `GET /metrics` in Prometheus text format — no exporter, no
sidecar. Sample series:

```
parkpulse_device_up{ip="192.168.1.64",name="Entrance cam"} 1
parkpulse_device_rtt_ms{ip="192.168.1.64",name="Entrance cam"} 2.4
parkpulse_device_jitter_ms{ip="192.168.1.64",name="Entrance cam"} 0.6
parkpulse_device_loss_ratio{ip="192.168.1.64",name="Entrance cam"} 0
parkpulse_passes_total 187
parkpulse_avg_latency_ms 842.3
parkpulse_ghost_openings_total 3
parkpulse_opens_total{kind="violation"} 2
parkpulse_cpu_percent{core="0"} 23.4
parkpulse_speedtest_download_mbps 92.4
```

**1. Point Prometheus at ParkPulse.** In `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: parkpulse
    scrape_interval: 15s
    static_configs:
      - targets: ["parkpulse-host:8888"]
```

Reload Prometheus; the `parkpulse` target should show **UP** on
`http://<prometheus>:9090/targets`.

**2. Add Prometheus as a data source in Grafana.** *Connections → Data sources
→ Add → Prometheus*, URL `http://<prometheus>:9090`, Save & test.

**3. Build panels.** Create a dashboard and add queries, for example:

- Gate reaction time (time series): `parkpulse_avg_latency_ms`
- Ghost openings (stat): `parkpulse_ghost_openings_total`
- Openings by type (bar/pie): `parkpulse_opens_total`
- Device availability (stat/table): `parkpulse_device_up`
- Camera latency & jitter (time series):
  `parkpulse_device_rtt_ms` and `parkpulse_device_jitter_ms`
- Packet loss alert: `parkpulse_device_loss_ratio > 0.1`

**4. Alerting (optional).** In Grafana *Alerting → Alert rules*, create a rule
such as `parkpulse_ghost_openings_total > 0` or
`parkpulse_device_up == 0 for 2m` and route it to Telegram, email, or a webhook.
This turns the dashboard from something you watch into something that pages you.

## Architecture

```
Docker logs ─► collector ─► parser ─► analyzer ─┐
                                                ├─► WebSocket hub ─► dashboard (Next.js)
LAN devices ─► netmon (ping + quality) ─────────┤
server stats ─► collector.health ───────────────┤
                                                └─► /metrics (Prometheus)
```

- **parser** — regex-matches log lines into typed events (ANPR, Gateway,
  Permit, POS, Open, Remote).
- **analyzer** — assembles events into per-car sessions, computes latency, and
  classifies each opening into one of the states above.
- **netmon** — pings monitored devices, scans subnets, fingerprints device type
  and vendor, and derives ping-quality stats over a rolling window.
- **ws** — fans out snapshots and live events to browsers; also renders
  `/metrics`.

## Development

```bash
# Backend
cd backend && go test ./...

# Frontend
cd frontend && npm install && npm run dev   # http://localhost:3000
```

The dev UI expects the backend WebSocket on the same host; run the backend with
`STATIC_DIR` pointing at `frontend/out` (after `npm run build`) to serve both
from one process, exactly as the Docker image does.

## License

MIT
