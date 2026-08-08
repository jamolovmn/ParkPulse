# ParkPulse

**Real-time monitoring for barrier-gate parking systems.**
🇺🇿 O'zbekcha to'liq hujjat: [**README.uz.md**](README.uz.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](backend/go.mod)
[![Next.js](https://img.shields.io/badge/Next.js-static%20UI-000000?logo=nextdotjs&logoColor=white)](frontend)
[![Docker image](https://img.shields.io/badge/Docker-~29MB%20Alpine-2496ED?logo=docker&logoColor=white)](Dockerfile)
[![Prometheus](https://img.shields.io/badge/Prometheus-%2Fmetrics-E6522C?logo=prometheus&logoColor=white)](#grafana--prometheus)
[![GitHub stars](https://img.shields.io/github/stars/jamolovmn/ParkPulse?style=flat)](https://github.com/jamolovmn/ParkPulse/stargazers)
[![Last commit](https://img.shields.io/github/last-commit/jamolovmn/ParkPulse)](https://github.com/jamolovmn/ParkPulse/commits/main)

ParkPulse tails the parking controller's Docker logs, reconstructs each car's
ANPR → payment → barrier-open chain, and surfaces two things operators actually
care about:

- **How fast** the system reacts (ANPR-to-payment latency, broken down per stage).
- **Why the barrier opened** — classified into four states, so a genuine
  "ghost opening" is never confused with a normal paid exit or a network hiccup.

It also monitors the LAN devices around the gate (cameras, relays, POS
terminals) with ping-quality metrics, watches server/container health, ships an
optional **AI agent** you can chat with to diagnose the host, and exposes
everything over WebSocket to a single-page dashboard and over a Prometheus
`/metrics` endpoint.

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

---

## Table of contents

- [What it does](#what-it-does) — every feature, explained
- [Quick start](#quick-start) — run it in one `docker run`
- [The AI agent](#the-ai-agent) — chat with your host
- [Configuration](#configuration) — every env var
- [Adaptive log reading](#adaptive-log-reading)
- [Alerting](#alerting)
- [SNMP monitoring](#snmp-switch--router-monitoring)
- [Grafana / Prometheus](#grafana--prometheus)
- [HTTP & WebSocket API](#http--websocket-api)
- [Architecture](#architecture)
- [Development](#development)

---

## What it does

Each feature below is independent — turn on only what you need.

### 1. Latency tracing
Every exit is reconstructed as a chain: **ANPR → Gateway → DB (permit) → POS
(payment)**. ParkPulse reports the total ANPR-to-payment time *and* a per-stage
breakdown, so you can see whether a slow reaction is the camera, the database, or
the POS terminal. Remote/auto-pay openings (where the driver's dwell time would
inflate the number) are flagged and **excluded from the average**, so the KPI
stays honest. Shown in the **Boshqaruv (Dashboard) → Jonli oqim (Live feed)** tab.

### 2. Four-state opening classifier
"The barrier opened" and "a car paid and left" are not the same event. Every
opening is sorted into one of these states — see [Why](#why-four-states) below:

| State | Meaning | Anomaly? |
|-------|---------|----------|
| **Paid** | Payment went through in software, then the barrier opened. | No |
| **Remote** | Guard opened with the remote; the system auto-charged on exit. | No |
| **Entry** | Car entered an `enter` gate (no payment expected). | No |
| **Violation** | Car on the sensor with a debt — opened with no payment, no remote. | **Yes** |
| **Ghost** | No car on the sensor at all, yet the barrier opened. | **Yes** |

Only **Violation** and **Ghost** increment the ghost counter and are saved with
surrounding log lines as evidence. Shown in the **Ochilishlar (Openings)** panel.

### 3. Adaptive log reading (no fixed vendor format, no AI)
Different controllers word their logs differently. ParkPulse handles this
deterministically and fully offline — multilingual gate keywords plus a
correlation detector that *learns* which log line means "barrier opened" by
watching what consistently follows a payment. See
[Adaptive log reading](#adaptive-log-reading). No regex edits required.

### 4. Live log inspector
The **Loglar (Logs)** view shows every raw log line with the label ParkPulse
gave it (ANPR / POS / OPEN / …), and marks auto-detected opens with `OPEN∗`, so
misclassifications are obvious at a glance.

### 5. 24-hour traffic chart
Hourly **entries vs. exits** over the last 24 hours, on the dashboard.

### 6. Network device monitoring
Scans the subnet, fingerprints each device (camera / web / unknown, plus vendor
like Hikvision or Dahua), and tracks **ping quality** per device: jitter, packet
loss, uptime %, min/avg/max RTT, and a live RTT sparkline. Works even for
devices that block ICMP (TCP fallback). Star (★) a device to be alerted when it
goes down; rename (✎) any device so it's unmistakable. Shown in the
**Qurilmalar (Devices)** tab.

### 7. Server & container health
CPU per core, RAM usage, uptime, and `docker stats` (CPU/RAM) per container.
Shown in the **Tizim (System)** tab.

### 8. SNMP switch / router monitoring
Polls managed switches/routers for interface **status** (up/down) and live
**throughput** (in/out Mbps). A **Tarmoq (Network)** tab appears when configured.
See [SNMP monitoring](#snmp-switch--router-monitoring).

### 9. Alerting (Telegram / webhook)
Pushes a notification the moment a watched device goes down, a ghost/violation
opening happens, or an SNMP port drops — no Grafana required. Fires **only on a
state change**, so a flapping link never spams you. See [Alerting](#alerting).

### 10. Internet speedtest
Periodic download / upload / ping via Cloudflare, shown in the header.

### 11. AI agent
An on-host assistant you can open like `claude` from the VPS (`pulse`) or from
the **Agent** tab in the dashboard. It can read logs, inspect containers, and fix
problems — with a safety gate on destructive commands. See
[The AI agent](#the-ai-agent).

### 12. Prometheus `/metrics`
Everything above is also exported in Prometheus text format for Grafana. See
[Grafana / Prometheus](#grafana--prometheus).

### <a id="why-four-states"></a>Why four states?
On a barrier gate, conflating "opened" with "paid" hides real problems. A
`Connection is closed` line from the relay hardware is treated as noise, **not**
an opening — that single rule removes the most common source of false ghost
alerts.

---

## Quick start

```bash
docker run -d --name parkpulse \
  -e TARGET_CONTAINER=p24gui \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /usr/local/bin:/host/bin \
  --network host \
  ghcr.io/jamolovmn/parking-pulse:latest
```

Then open **`http://localhost:8888`**.

What each line does:

- **`-e TARGET_CONTAINER=p24gui`** — the container whose logs to read
  (comma-separated for several). **Optional** — you can also pick the
  container(s) live from the dashboard (**Tizim → Kuzatiladigan konteyner**); the
  choice is saved (`TARGET_STORE`, default `target.json`) and applied without a
  restart.
- **`-v /var/run/docker.sock:/var/run/docker.sock`** — lets ParkPulse list
  containers, tail logs, and read `docker stats`. Required.
- **`-v /usr/local/bin:/host/bin`** — on start the container drops two host
  commands, **`pulse`** and **`parkpulse`**, so you can open the AI agent CLI
  from the VPS just like `claude`. Skip this mount and the server still runs; you
  just won't get the host shortcuts. `PULSE_HOST_BIN` overrides the in-container
  target path (default `/host/bin`).
- **`--network host`** — what makes LAN device scanning and pinging work.

Once running, from the host:

```bash
pulse                          # opens the interactive AI agent (like `claude`)
PULSE_PASSWORD='parol' pulse   # if you set an agent password
```

No mount? Install the shortcuts manually instead: `sudo ./install-cli.sh`.

### Build it yourself

```bash
./build.sh --local   # build the image locally (no push)
```

The [Dockerfile](Dockerfile) builds the Next.js UI to static files and the Go
binary, then ships a ~29 MB Alpine image.

---

## The AI agent

ParkPulse bundles an LLM-powered assistant that lives **on the parking host**. It
is not a chatbot bolted on the side — it can actually read the parking logs,
inspect and restart containers, and edit files, then verify its own fix. Think of
it as `claude` that already knows this specific deployment (it reads
[`AGENT.md`](AGENT.md) as ground truth).

**Two ways to use it:**

1. **From the VPS terminal** — just type `pulse` (or `parkpulse`). Same feel as
   the `claude` CLI: an interactive prompt with history and arrow-key editing.
2. **From the dashboard** — the **Agent** tab gives you the same assistant in the
   browser, with streaming replies.

**What it can do** (its tools): run shell commands (`bash`), read container logs
(`docker_logs`), list containers (`docker_ps`), restart a container
(`docker_restart`), and read/write files (`read_file` / `write_file`).

**Safety — human in the loop.** Read-only actions and simple config edits run
automatically. **Destructive** commands (deleting files, `docker rm/stop/kill`,
`DROP`/`DELETE`, `mkfs`, `shutdown`, force-push, recursive `chmod` …) always stop
and **ask for confirmation first** — the agent never runs them on its own.

**Providers.** Point it at whichever LLM you have a key for — configured live in
the **Agent → Sozlamalar (Settings)** panel (no rebuild):

| Provider | Default model |
|----------|---------------|
| `anthropic` | `claude-opus-4-8` |
| `openai` | `gpt-4o` |
| `openrouter` | `anthropic/claude-opus-4-8` |
| `nvidia` | `meta/llama-3.1-70b-instruct` |
| `local` (Ollama etc.) | `llama3.1` (via `http://localhost:11434/v1`) |

Any OpenAI-compatible endpoint works via `openai` + a custom `base_url`.

**Protect it.** Set `AGENT_PASSWORD` (env) so only people with the password can
talk to the agent or change its settings; then `PULSE_PASSWORD='…' pulse` from
the host. A separate **sudo password** can be stored so the agent can run
privileged fixes when you allow them.

Example — ask it *"why did the exit gate container restart?"* and it will run the
crash-investigation procedure from `AGENT.md`: inspect exit code / OOM state,
grep the last 300 log lines, check host memory, and quote the exact line that
proves the cause before proposing a fix.

---

## Configuration

Every setting is an **environment variable**, and explicit env always wins. A
YAML file is an optional convenience — see
[`parkpulse.example.yaml`](parkpulse.example.yaml). Copy it to `parkpulse.yaml`
(working dir), or `/etc/parkpulse/config.yaml`, or point `CONFIG_FILE` at it.

### Core

| Env | Default | Purpose |
|-----|---------|---------|
| `TARGET_CONTAINER` | — | Container name(s) to tail, comma-separated. Optional — can also be chosen from the dashboard. |
| `LISTEN_ADDR` | `:8888` | HTTP/WebSocket listen address. |
| `STATIC_DIR` | embedded | Path to the built UI (only needed in dev). |
| `CONFIG_FILE` | — | Explicit path to a YAML config. |

### Devices & network

| Env | Default | Purpose |
|-----|---------|---------|
| `DEVICES` | — | Monitored devices: `name=ip,name=ip`. |
| `SCAN_SUBNET` | auto | Subnet(s) for the scanner, e.g. `192.168.1.0/24`. |
| `SPEEDTEST_MIN` | `15` | Speedtest interval in minutes (`0` disables). |

### Opening / latency analyzer

| Env | Default | Purpose |
|-----|---------|---------|
| `MATCH_WINDOW_SEC` | `180` | ANPR→payment correlation window. |
| `AUTOPAY_SEC` | `90` | How long to wait for auto-payment after an opening (remote vs. violation). |
| `PRESENCE_SEC` | `60` | How long an ANPR read counts as "car on the sensor". |
| `GRACE_SEC` | `3` | How long to wait for a late ANPR before deciding a ghost. |
| `DEDUPE_SEC` | `60` | Suppress duplicate lines for the same plate. |
| `GATE_DEDUPE_SEC` | `10` | Suppress duplicate hardware lines on the same gate. |
| `RELAY_OPEN_RE` | built-in | Regex for the physical barrier-open log line. |
| `RELAY_REMOTE_RE` | built-in | Regex for the guard's remote-open signal. |
| `GATE_ENTER_WORDS` | `enter,entry,kirish,in` | Words that mean an entry lane (any language). |
| `GATE_EXIT_WORDS` | `exit,chiqish,out` | Words that mean an exit lane. |
| `OPEN_LEARN_WINDOW_SEC` | `8` | Max gap after a payment for a line to count as the "open" line. |
| `OPEN_LEARN_MIN` | `5` | Correlated occurrences before an "open" template is trusted. |
| `OPEN_LEARN_RATIO` | `0.6` | Fraction of a template's occurrences that must follow a payment. |

### SNMP

| Env | Default | Purpose |
|-----|---------|---------|
| `SNMP_TARGETS` | — | SNMP devices: `name=ip@community`, comma-separated. Add `#1` for SNMP v1. |
| `SNMP_INTERVAL_SEC` | `30` | SNMP poll interval. |

### Alerting

| Env | Default | Purpose |
|-----|---------|---------|
| `ALERT_TELEGRAM_TOKEN` | — | Telegram bot token (from @BotFather). |
| `ALERT_TELEGRAM_CHAT` | — | Telegram chat/channel id to send alerts to. |
| `ALERT_WEBHOOK_URL` | — | Optional URL to POST a JSON alert payload to. |

### AI agent & storage

| Env | Default | Purpose |
|-----|---------|---------|
| `AGENT_PASSWORD` | — | Password required to use/configure the AI agent. |
| `PULSE_HOST_BIN` | `/host/bin` | Where the `pulse`/`parkpulse` shortcuts are dropped. |
| `TARGET_STORE` | `target.json` | Where the chosen container(s) are saved. |
| `DEVICES_STORE` | `devices.json` | Where watched/renamed devices are saved. |
| `ALERT_STORE` | `alerts.json` | Where alert settings are saved. |

> Mount the `*.json` stores on a volume if you want dashboard-saved settings to
> survive a re-pull.

---

## Adaptive log reading

Different controllers/sites word their logs differently, so a single fixed regex
misses events on some installations. ParkPulse handles this **deterministically —
no AI, fully offline** — in three layers:

1. **Multilingual gate words.** Entry/exit are matched from a configurable word
   list (`GATE_ENTER_WORDS` / `GATE_EXIT_WORDS`) — `exit 1`, `chiqish 1`, `out 3`
   all normalize to the same canonical gate.
2. **Correlation learning.** Each unmatched line is reduced to a *template*
   (numbers and plates → `#`). The template that consistently appears within a
   few seconds **after a payment** is learned to be the "barrier opened" line —
   ParkPulse then treats it as an open event even though no regex matched it.
3. **Direction from behaviour.** A learned open that follows a payment is an
   **exit**; one that follows only a plate read (no payment) is an **entry**.

Watch it work in the **Loglar (Logs)** tab: every line shows the label ParkPulse
gave it, `OPEN∗` marks an auto-detected open, and a banner shows the learned
template. Nothing is sent anywhere — the learning happens in-process.

---

## Alerting

ParkPulse can push a notification the moment something goes wrong — no Grafana
required. Alerts fire **only on a state change** (a device goes down, then a
separate alert when it recovers), so a flapping link doesn't spam you.

**Triggers:**

- **Device down / recovered** — a **watched** device stops (or resumes)
  responding. Only devices you star (★) in **Qurilmalar (Devices)** alert, so
  transient phones/laptops on the LAN never page you. Devices listed in `DEVICES`
  are watched by default; auto-scanned ones are not. You can also **rename** any
  device (✎). Both are saved (`DEVICES_STORE`).
- **Ghost / violation opening** — a suspicious barrier opening.
- **SNMP port down / up** — a switch interface changes state.

**Configure it from the dashboard** — no rebuild. Open **Tizim (System) →
Ogohlantirish**, paste your Telegram bot token + chat id (and/or a webhook URL),
**Save**, then **Send test**. Settings are written to a JSON file (`ALERT_STORE`)
and survive a restart.

- **Telegram** — create a bot with [@BotFather](https://t.me/BotFather) for the
  token; the chat id is your channel/group id (or personal chat id).
- **Webhook** — ParkPulse POSTs `{level, title, text, time}` to the URL (route it
  to Slack, a script, anything).

The same values can be set via env (`ALERT_TELEGRAM_TOKEN`, `ALERT_TELEGRAM_CHAT`,
`ALERT_WEBHOOK_URL`) or the `alerts:` block in YAML. A value saved from the UI
takes precedence over env on the next restart.

---

## SNMP (switch / router monitoring)

Point ParkPulse at managed switches or routers and it polls each interface for
**operational status** (up/down) and **live throughput** (in/out Mbps, derived
from the interface octet counters). A **Tarmoq (Network)** tab appears in the
dashboard, and the data is also exported to Prometheus.

```bash
-e SNMP_TARGETS="Core=192.168.1.1@public,Edge=192.168.1.2@public"
```

Format is `name=ip@community`, comma-separated. Append `#1` for SNMP v1
(`...@public#1`); the default is v2c. Throughput needs two polls to appear.
Requires SNMP enabled on the device (read-only community is enough).

---

## Grafana / Prometheus

### How the pieces fit (read this first)

Three separate programs, connected in a chain:

```
ParkPulse (/metrics)  ──►  Prometheus  ──►  Grafana
   raw numbers,             stores them        draws
   "right now"              over time          graphs
```

**ParkPulse does not appear in any "apps" list inside Grafana or Prometheus.**
You do not register it anywhere. It simply publishes a plain-text page at
`http://<host>:8888/metrics`. You wire the sensor to the recorder by **editing
Prometheus's config file**; then Grafana connects to *Prometheus* (not to
ParkPulse). Mental model: *ParkPulse is the sensor, Prometheus is the recorder,
Grafana is the screen.*

### Step by step

A ready-to-run setup lives in [`monitoring/`](monitoring/).

**1. Run Prometheus + Grafana.**

```bash
docker compose -f monitoring/docker-compose.yml up -d
```

**2. Tell Prometheus where ParkPulse is.** Edit
[`monitoring/prometheus.yml`](monitoring/prometheus.yml):

```yaml
scrape_configs:
  - job_name: parkpulse
    static_configs:
      - targets: ["host.docker.internal:8888"]   # or the LAN IP, e.g. 192.168.1.50:8888
```

> **The #1 mistake:** do not write `localhost:8888` here. Prometheus runs in its
> own container, so `localhost` means *Prometheus itself*. Use
> `host.docker.internal:8888` or the server's real LAN IP, then
> `docker compose -f monitoring/docker-compose.yml restart prometheus`.

**3. Verify.** Open `http://localhost:9090/targets`. The `parkpulse` target must
say **UP**.

**4. Connect Grafana to Prometheus.** Open `http://localhost:3000`
(login `admin` / `admin`) → **Connections → Data sources → Add data source** →
pick **Prometheus** → URL `http://parkpulse-prometheus:9090` → **Save & test**.

**5. Build panels.** **Dashboards → New → Add visualization**, choose the
Prometheus data source, type a metric name:

| Panel | Query |
|-------|-------|
| Gate reaction time | `parkpulse_avg_latency_ms` |
| Ghost openings | `parkpulse_ghost_openings_total` |
| Openings by type | `parkpulse_opens_total` |
| Devices online | `parkpulse_device_up` |
| Camera latency / jitter | `parkpulse_device_rtt_ms` · `parkpulse_device_jitter_ms` |
| Packet loss | `parkpulse_device_loss_ratio` |
| Switch port up | `parkpulse_snmp_if_up` |
| Switch throughput | `parkpulse_snmp_if_in_mbps` · `parkpulse_snmp_if_out_mbps` |

### Sample `/metrics`

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
parkpulse_snmp_if_up{host="Core switch",if="Gi0/1"} 1
parkpulse_snmp_if_in_mbps{host="Core switch",if="Gi0/1"} 143.2
parkpulse_speedtest_download_mbps 92.4
```

---

## HTTP & WebSocket API

All served from the same `LISTEN_ADDR` (default `:8888`):

| Path | Method | Purpose |
|------|--------|---------|
| `/` | GET | The dashboard (embedded static UI). |
| `/ws` | WS | Live snapshot + event stream to the browser. |
| `/healthz` | GET | Liveness check. |
| `/metrics` | GET | Prometheus metrics. |
| `/api/logs` | GET | Recent labelled log lines (for the inspector). |
| `/api/containers` | GET | List of running containers. |
| `/api/target` | GET/POST | Get/set the watched container(s). |
| `/api/scan` | POST | Trigger a subnet scan. |
| `/api/devices/watch` | POST | Star/unstar a device for alerts. |
| `/api/devices/name` | POST | Rename a device. |
| `/api/alerts` | GET/POST | Get/set alert settings. |
| `/api/alerts/test` | POST | Send a test alert. |
| `/api/agent/login` | POST | Authenticate to the agent. |
| `/api/agent/stream` | GET | Streamed agent replies. |
| `/api/agent/config` | GET/POST | Get/set agent provider settings. |
| `/api/agent/models` | GET | List models for the configured provider. |
| `/api/agent/test` | POST | Validate the agent's API key. |

---

## Architecture

```
Docker logs ─► collector ─► parser ─► analyzer ─┐
LAN devices ─► netmon (ping + quality) ─────────┤
switches ────► snmp (interface poll) ───────────├─► WebSocket hub ─► dashboard (Next.js)
server stats ─► collector.health ───────────────┤                └─► /metrics (Prometheus)
host shell ──► agent (LLM + tools, guarded) ────┤
                                                └─► alert (Telegram / webhook)
```

- **parser** — regex-matches log lines into typed events (ANPR, Gateway, Permit,
  POS, Open, Remote).
- **analyzer** — assembles events into per-car sessions, computes latency, and
  classifies each opening.
- **detector** — the adaptive layer that learns the "open" line by correlation.
- **netmon** — pings devices, scans subnets, fingerprints type/vendor, derives
  ping-quality stats.
- **snmp** — polls managed switches/routers.
- **agent** — the on-host LLM assistant with a human-in-the-loop guard.
- **alert** — pushes Telegram/webhook alerts on state changes.
- **ws** — fans out snapshots/events to browsers; renders `/metrics`.

---

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
