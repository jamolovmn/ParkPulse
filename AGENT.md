# ParkPulse Agent — deployment notes

You are operating on the host of a **smart-parking** deployment. ParkPulse itself is
a monitoring tool that tails the parking application's Docker logs and analyzes them.
Use this file as ground truth about this specific system. The operator can edit it.

## Containers you will see
- **The parking app** — the container whose logs ParkPulse reads. Its name varies per
  site (e.g. `p24gui`, `parking24-gateway-api`); it is set via the `TARGET_CONTAINER`
  env or chosen in the UI. This is usually the one that matters.
- **postgres** — the parking database. Do NOT connect to it directly; you only have
  Docker-level access (logs, stats, restart). Diagnose via logs, not SQL.
- **parkpulse** — this monitoring tool itself.

Always run `docker ps` first to see the real names on THIS host.

## How to investigate a crashed / restarting container
Do all of these before concluding — never stop at "it restarted":
1. `docker inspect <name> --format '{{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} restarts={{.RestartCount}} started={{.State.StartedAt}}'`
2. `docker logs --tail 300 --timestamps <name>` and grep for `error|fatal|panic|killed|exception`
3. If `OOMKilled=true` or exit 137 → memory. Check `free -m`, `docker stats --no-stream`,
   and the host: `dmesg | grep -i -E 'oom|killed process'`.
4. Exit 1 / app panic → read the stack/error lines around the last start timestamp.
5. Only after you have the actual cause (with the log line as evidence) do you propose
   or apply a fix, then **verify**: restart if needed and confirm it stays healthy.

## Parking log format (the app's own logs)
Lines look like: `20260703 13:00:29.100000 UTC 1 DEBUG [openGate] Relay exit 1: ...`
- ANPR (plate read): `[operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc`
- Gate open: `[openGate] Relay <enter|exit> N: ...` (empty body = hardware/ghost open)
- Payment: `[makePayment] Vendotek <gate>: Requesting payment: <plate> (<amount>)`
- A "ghost" opening = gate opened with no car on the sensor (suspicious).
When asked about parking events, grep the app container logs for these patterns.

## Common asks & the right commands
- "why did X restart" → the crash procedure above.
- "high CPU/RAM" → `docker stats --no-stream`, then `docker top <name>` / logs.
- "disk full" → `df -h`, `du -xh --max-depth=1 / | sort -h | tail`.
- "gate not opening" → grep app logs around the time for Relay lines + errors.

## Rules of thumb
- Prefer `docker_logs` / `docker inspect` over guessing.
- Read real evidence; quote the exact log line that proves the cause.
- Make minimal changes, then verify. Reply in the user's language (Uzbek).
