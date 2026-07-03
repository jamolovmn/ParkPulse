'use client';

import { useState } from 'react';
import { useParkPulse, Pass, Ghost, Stats } from '@/lib/useParkPulse';

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString('uz-UZ', { hour12: false });

const fmtMs = (ms: number) => `${Math.round(ms)} ms`;

export default function Dashboard() {
  const { connected, stats, passes, ghosts } = useParkPulse();

  return (
    <main className="mx-auto max-w-6xl px-6 py-8">
      <Header connected={connected} />
      <KpiRow stats={stats} />
      <div className="mt-6 grid gap-6 lg:grid-cols-3">
        <LiveFeed passes={passes} />
        <GhostList ghosts={ghosts} />
      </div>
    </main>
  );
}

function Header({ connected }: { connected: boolean }) {
  return (
    <header className="mb-8 flex items-center justify-between">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">ParkPulse</h1>
        <p className="mt-0.5 text-sm text-ink-muted">Smart Parking Monitoring</p>
      </div>
      <div className="flex items-center gap-2 rounded-full border border-line px-3 py-1.5 text-xs text-ink-secondary">
        <span
          className={`h-2 w-2 rounded-full ${connected ? 'bg-good' : 'bg-critical'}`}
        />
        {connected ? 'Jonli' : 'Uzilgan'}
      </div>
    </header>
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
                  <tr
                    key={key}
                    onClick={() => p.breakdown && setOpen(expanded ? null : key)}
                    className={`border-t border-grid ${
                      p.breakdown ? 'cursor-pointer hover:bg-white/[0.03]' : ''
                    }`}
                  >
                    <td className="px-5 py-2.5 align-top text-ink-secondary">
                      {fmtTime(p.relay_at)}
                    </td>
                    <td className="px-5 py-2.5 align-top font-medium">{p.plate}</td>
                    <td className="px-5 py-2.5 align-top text-ink-secondary">
                      {p.gate || '—'}
                    </td>
                    <td className="px-5 py-2.5 text-right">
                      <span
                        className={p.latency_ms > 1500 ? 'text-warn' : 'text-ink-secondary'}
                      >
                        {fmtMs(p.latency_ms)}
                      </span>
                      {expanded && p.breakdown && (
                        <div className="mt-1 whitespace-nowrap text-xs text-ink-muted">
                          DB: {fmtMs(p.breakdown.db_ms)} · Logic: {fmtMs(p.breakdown.logic_ms)}
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function GhostList({ ghosts }: { ghosts: Ghost[] }) {
  return (
    <section className="rounded-lg border border-line bg-surface">
      <h2 className="border-b border-line px-5 py-3.5 text-sm font-medium">
        Arvoh ochilishlar
      </h2>
      {ghosts.length === 0 ? (
        <Empty text="Arvoh ochilish yo'q" />
      ) : (
        <ul className="divide-y divide-grid">
          {ghosts.map((g, i) => (
            <li key={`${g.relay_at}-${i}`} className="px-5 py-3">
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
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Empty({ text }: { text: string }) {
  return <p className="px-5 py-10 text-center text-sm text-ink-muted">{text}</p>;
}
