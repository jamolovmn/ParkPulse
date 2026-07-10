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

![ParkPulse dashboard](docs/dashboard.png)

<details>
<summary>More screenshots</summary>

**Opening history — four states, only the anomalies are logged**

![Openings](docs/openings.png)

**Network devices — ping quality (jitter, loss, uptime) with sparklines**

![Devices](docs/devices.png)

</details>

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

### How the pieces fit (read this first)

There are three separate programs, and they connect in a chain:

```
ParkPulse (/metrics)  ──►  Prometheus  ──►  Grafana
   raw numbers,             stores them        draws
   "right now"              over time          graphs
```

**ParkPulse does not appear in any "apps" or "integrations" list inside Grafana
or Prometheus.** You do not register it anywhere. It simply publishes a plain
text page at `http://<host>:8888/metrics`. The connection happens by **editing
Prometheus's config file** to tell Prometheus that URL — that is the whole
"adding" step. Then Grafana connects to *Prometheus* (not to ParkPulse).

So the mental model is: *ParkPulse is the sensor, Prometheus is the recorder,
Grafana is the screen.* You wire the sensor to the recorder in a config file,
and you pick the recorder from Grafana's data-source list.

### Step by step

A ready-to-run setup lives in [`monitoring/`](monitoring/).

**1. Run Prometheus + Grafana.** From the repo:

```bash
docker compose -f monitoring/docker-compose.yml up -d
```

**2. Tell Prometheus where ParkPulse is.** Edit
[`monitoring/prometheus.yml`](monitoring/prometheus.yml) and set the target to
the host where ParkPulse runs:

```yaml
scrape_configs:
  - job_name: parkpulse
    static_configs:
      - targets: ["host.docker.internal:8888"]   # or the LAN IP, e.g. 192.168.1.50:8888
```

> **The #1 mistake:** do not write `localhost:8888` here. Prometheus runs in its
> own container, so `localhost` means *Prometheus itself*, not ParkPulse. Use
> `host.docker.internal:8888` (works on the provided compose) or the server's
> real LAN IP. Restart with `docker compose -f monitoring/docker-compose.yml restart prometheus`.

**3. Verify the connection.** Open `http://localhost:9090/targets`. The
`parkpulse` target must say **UP**. If it says DOWN, the target address is wrong
(see the mistake above) or ParkPulse isn't reachable from the Prometheus
container.

**4. Connect Grafana to Prometheus.** Open `http://localhost:3000`
(login `admin` / `admin`). Go to **Connections → Data sources → Add data
source**, and from the list **pick "Prometheus"** — *this is the step you were
looking for; you choose Prometheus here, ParkPulse is never in this list.* Set
the URL to `http://parkpulse-prometheus:9090` (the compose service name) and
click **Save & test**.

**5. Build panels.** **Dashboards → New → Add visualization**, choose the
Prometheus data source, and type a metric name into the query box. Examples:

| Panel | Query |
|-------|-------|
| Gate reaction time | `parkpulse_avg_latency_ms` |
| Ghost openings | `parkpulse_ghost_openings_total` |
| Openings by type | `parkpulse_opens_total` |
| Devices online | `parkpulse_device_up` |
| Camera latency / jitter | `parkpulse_device_rtt_ms` · `parkpulse_device_jitter_ms` |
| Packet loss | `parkpulse_device_loss_ratio` |

**6. Alerting (optional).** In Grafana **Alerting → Alert rules**, create a rule
like `parkpulse_ghost_openings_total > 0` or `parkpulse_device_up == 0 for 2m`
and route it to Telegram, email, or a webhook — so the dashboard pages you
instead of you watching it.

### The metrics

`GET /metrics` in Prometheus text format, no exporter needed. Sample:

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
