'use client';

import { Fragment, useState } from 'react';
import { useParkPulse, Breakdown, Device, Pass, Ghost, Stats } from '@/lib/useParkPulse';

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString('uz-UZ', { hour12: false });

// Zanjir qadamlari juda tez (0.1-15ms) — kichik qiymatlarda kasr ko'rsatamiz
const fmtMs = (ms: number) => `${ms < 10 ? ms.toFixed(1) : String(Math.round(ms))} ms`;

export default function Dashboard() {
  const { connected, stats, passes, ghosts, devices } = useParkPulse();

  return (
    <main className="mx-auto max-w-6xl px-6 py-8">
      <Header connected={connected} />
      <KpiRow stats={stats} />
      <div className="mt-6 grid gap-6 lg:grid-cols-3">
        <LiveFeed passes={passes} />
        <div className="space-y-6 self-start">
          <GhostList ghosts={ghosts} />
          <DevicePanel devices={devices} />
        </div>
      </div>
    </main>
  );
}

function Header({ connected }: { connected: boolean }) {
  return (
    <header className="mb-8 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">ParkPulse</h1>
        <p className="mt-0.5 text-sm text-ink-muted">Smart Parking Monitoring</p>
      </div>
      <div className="flex items-center gap-3">
        <SpeedTest />
        <div className="flex items-center gap-2 rounded-full border border-line px-3 py-1.5 text-xs text-ink-secondary">
          <span
            className={`h-2 w-2 rounded-full ${connected ? 'bg-good' : 'bg-critical'}`}
          />
          {connected ? 'Jonli' : 'Uzilgan'}
        </div>
      </div>
    </header>
  );
}

type SpeedResult = { ping_ms: number; download_mbps: number; upload_mbps: number };

function SpeedTest() {
  const [running, setRunning] = useState(false);
  const [res, setRes] = useState<SpeedResult | null>(null);
  const [err, setErr] = useState(false);

  const run = async () => {
    setRunning(true);
    setErr(false);
    try {
      const r = await fetch('/api/speedtest');
      if (!r.ok) throw new Error();
      setRes(await r.json());
    } catch {
      setErr(true);
      setRes(null);
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="flex items-center gap-3">
      {res && !running && (
        <span className="text-xs text-ink-secondary [font-variant-numeric:tabular-nums]">
          ↓ {res.download_mbps.toFixed(1)} Mbps · ↑ {res.upload_mbps.toFixed(1)} Mbps ·{' '}
          {Math.round(res.ping_ms)} ms
        </span>
      )}
      {err && !running && <span className="text-xs text-critical">Test xato</span>}
      <button
        onClick={run}
        disabled={running}
        className="rounded-md border border-line px-3 py-1.5 text-xs text-ink-secondary transition-colors hover:bg-white/[0.05] disabled:opacity-50"
      >
        {running ? 'O‘lchanmoqda…' : 'Internet tezligi'}
      </button>
    </div>
  );
}

function DevicePanel({ devices }: { devices: Device[] }) {
  const [scanning, setScanning] = useState(false);
  const [scanErr, setScanErr] = useState<string | null>(null);

  const scan = async () => {
    setScanning(true);
    setScanErr(null);
    try {
      const r = await fetch('/api/scan', { method: 'POST' });
      if (!r.ok) setScanErr(await r.text());
    } catch {
      setScanErr('So‘rov yuborilmadi');
    } finally {
      setScanning(false);
    }
  };

  return (
    <section className="rounded-lg border border-line bg-surface">
      <div className="flex items-center justify-between border-b border-line px-5 py-2.5">
        <h2 className="text-sm font-medium">Tarmoq qurilmalari</h2>
        <button
          onClick={scan}
          disabled={scanning}
          className="rounded-md border border-line px-2.5 py-1 text-xs text-ink-secondary transition-colors hover:bg-white/[0.05] disabled:opacity-50"
        >
          {scanning ? 'Qidirilmoqda…' : 'Qidirish'}
        </button>
      </div>
      {scanErr && (
        <p className="border-b border-grid px-5 py-2 text-xs text-warn">{scanErr}</p>
      )}
      {devices.length === 0 ? (
        <Empty text="Qurilma yo'q — «Qidirish»ni bosing yoki DEVICES env bering" />
      ) : (
        <ul className="divide-y divide-grid">
          {devices.map((d) => (
            <li key={d.ip} className="flex items-center gap-2.5 px-5 py-2.5 text-sm">
              <span
                className={`h-2 w-2 shrink-0 rounded-full ${d.alive ? 'bg-good' : 'bg-critical'}`}
              />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate font-medium">{d.name}</span>
                  {d.type && (
                    <span className="shrink-0 rounded border border-line px-1.5 py-0.5 text-[10px] text-ink-muted">
                      {d.type}
                      {d.vendor ? ` · ${d.vendor}` : ''}
                    </span>
                  )}
                </div>
                {d.name !== d.ip && (
                  <span className="text-xs text-ink-muted">{d.ip}</span>
                )}
              </div>
              <span
                className={`shrink-0 text-xs [font-variant-numeric:tabular-nums] ${
                  d.alive ? 'text-ink-secondary' : 'text-critical'
                }`}
              >
                {d.alive ? fmtMs(d.rtt_ms) : 'uzilgan'}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function KpiRow({ stats }: { stats: Stats }) {
  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <Kpi label="Jami kirishlar" value={String(stats.total_passes)} />
      <Kpi
        label="O'rtacha latency"
        value={stats.total_passes ? fmtMs(stats.avg_latency_ms) : '—'}
      />
      <Kpi
        label="Arvoh ochilishlar"
        value={String(stats.ghost_count)}
        alert={stats.ghost_count > 0}
      />
    </div>
  );
}

function Kpi({ label, value, alert }: { label: string; value: string; alert?: boolean }) {
  return (
    <div className="rounded-lg border border-line bg-surface p-5">
      <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">{label}</p>
      <p className={`mt-2 text-3xl font-semibold ${alert ? 'text-critical' : 'text-ink'}`}>
        {value}
      </p>
    </div>
  );
}

function LiveFeed({ passes }: { passes: Pass[] }) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <section className="rounded-lg border border-line bg-surface lg:col-span-2">
      <h2 className="border-b border-line px-5 py-3.5 text-sm font-medium">
        Jonli oqim
      </h2>
      {passes.length === 0 ? (
        <Empty text="Hodisalar kutilmoqda…" />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm [font-variant-numeric:tabular-nums]">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-ink-muted">
                <th className="px-5 py-2.5 font-medium">Vaqt</th>
                <th className="px-5 py-2.5 font-medium">Raqam</th>
                <th className="px-5 py-2.5 font-medium">Darvoza</th>
                <th className="px-5 py-2.5 text-right font-medium">Latency</th>
              </tr>
            </thead>
            <tbody>
              {passes.map((p, i) => {
                const key = `${p.relay_at}-${p.plate}-${i}`;
                const expanded = open === key;
                return (
                  <Fragment key={key}>
                    <tr
                      onClick={() => p.breakdown && setOpen(expanded ? null : key)}
                      className={`border-t border-grid ${
                        p.breakdown ? 'cursor-pointer hover:bg-white/[0.03]' : ''
                      }`}
                    >
                      <td className="px-5 py-2.5 text-ink-secondary">{fmtTime(p.relay_at)}</td>
                      <td className="px-5 py-2.5 font-medium">{p.plate}</td>
                      <td className="px-5 py-2.5 text-ink-secondary">{p.gate || '—'}</td>
                      <td
                        className={`px-5 py-2.5 text-right ${
                          p.latency_ms > 1500 ? 'text-warn' : 'text-ink-secondary'
                        }`}
                      >
                        {fmtMs(p.latency_ms)}
                      </td>
                    </tr>
                    {expanded && p.breakdown && (
                      <tr className="bg-white/[0.02]">
                        <td colSpan={4} className="px-5 pb-3 pt-1">
                          <Chain breakdown={p.breakdown} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

// Zanjir qadamlari: ANPR —ms→ Gateway —ms→ DB —ms→ Relay
function Chain({ breakdown: b }: { breakdown: Breakdown }) {
  const nodes = ['ANPR', 'Gateway', 'DB', 'Relay'];
  const times = [b.gateway_ms, b.db_ms, b.pos_ms];
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs [font-variant-numeric:tabular-nums]">
      {nodes.map((n, i) => (
        <Fragment key={n}>
          <span className="rounded-md border border-line px-2 py-0.5 font-medium text-ink-secondary">
            {n}
          </span>
          {i < times.length && (
            <span className="flex items-center gap-1 text-ink-muted">
              <span aria-hidden>—</span>
              <span>{fmtMs(times[i])}</span>
              <span aria-hidden>→</span>
            </span>
          )}
        </Fragment>
      ))}
    </div>
  );
}

function GhostList({ ghosts }: { ghosts: Ghost[] }) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <section className="rounded-lg border border-line bg-surface">
      <h2 className="border-b border-line px-5 py-3.5 text-sm font-medium">
        Arvoh ochilishlar
      </h2>
      {ghosts.length === 0 ? (
        <Empty text="Arvoh ochilish yo'q" />
      ) : (
        <ul className="divide-y divide-grid">
          {ghosts.map((g, i) => {
            const key = `${g.relay_at}-${i}`;
            const expanded = open === key;
            const hasContext = (g.context?.length ?? 0) > 0;
            return (
              <li
                key={key}
                onClick={() => hasContext && setOpen(expanded ? null : key)}
                className={`px-5 py-3 ${hasContext ? 'cursor-pointer hover:bg-white/[0.03]' : ''}`}
              >
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-critical" aria-hidden>
                    ▲
                  </span>
                  <span className="font-medium text-critical">ANPR'siz ochilish</span>
                  <span className="ml-auto text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
                    {fmtTime(g.relay_at)}
                  </span>
                </div>
                <p className="mt-1 text-xs text-ink-secondary">
                  Darvoza: {g.gate || 'noma’lum'}
                  {g.plate ? ` · Raqam: ${g.plate}` : ''}
                </p>
                <p className="mt-1 truncate font-mono text-xs text-ink-muted" title={g.raw}>
                  {g.raw}
                </p>
                {hasContext && (
                  <p className="mt-1.5 text-xs text-ink-muted">
                    {expanded ? '▾ Kontekst (o‘sha paytdagi loglar)' : '▸ Kontekstni ko‘rish'}
                  </p>
                )}
                {expanded && hasContext && (
                  <pre className="mt-2 max-h-48 overflow-auto rounded-md border border-grid bg-page p-3 text-[11px] leading-relaxed text-ink-muted">
                    {g.context!.join('\n')}
                  </pre>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function Empty({ text }: { text: string }) {
  return <p className="px-5 py-10 text-center text-sm text-ink-muted">{text}</p>;
}
