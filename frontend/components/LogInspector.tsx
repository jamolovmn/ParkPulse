'use client';

import { useEffect, useRef, useState } from 'react';

type LogLine = {
  time: string;
  container: string;
  text: string;
  kind: string;
  plate?: string;
  gate?: string;
};
type Learned = { container: string; template: string; count: number; ratio: number; sample: string };

// Yorliq ranglari. Tanilmagan ("") va shovqin — bosiq (ular normal, ko'p bo'ladi).
const TAG: Record<string, { label: string; cls: string }> = {
  ANPR: { label: 'ANPR', cls: 'text-accent border-accent/40' },
  POS: { label: 'POS', cls: 'text-good border-good/40' },
  OPEN: { label: 'OPEN', cls: 'text-warn border-warn/40' },
  'OPEN*': { label: 'OPEN∗', cls: 'text-warn border-warn/50 bg-warn/10' },
  REMOTE: { label: 'PULT', cls: 'text-accent border-accent/40' },
  GATEWAY: { label: 'GW', cls: 'text-ink-muted border-line' },
  PERMIT: { label: 'DB', cls: 'text-ink-muted border-line' },
  NOISE: { label: 'shovqin', cls: 'text-ink-muted/50 border-line' },
  '': { label: '—', cls: 'text-ink-muted/50 border-line' },
};

const EVENT_KINDS = new Set(['ANPR', 'POS', 'OPEN', 'OPEN*', 'REMOTE', 'GATEWAY', 'PERMIT']);

type Filter = 'all' | 'events' | 'other';
const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'Hammasi' },
  { id: 'events', label: 'Hodisalar' },
  { id: 'other', label: 'Tanilmagan' },
];

const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('uz-UZ', { hour12: false });

export default function LogInspector() {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [learned, setLearned] = useState<Learned[]>([]);
  const [filter, setFilter] = useState<Filter>('all');
  const [err, setErr] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setInterval>>();

  useEffect(() => {
    const load = async () => {
      try {
        const r = await fetch('/api/logs');
        if (!r.ok) throw new Error();
        const d = await r.json();
        setLines(d.lines ?? []);
        setLearned(d.learned ?? []);
        setErr(null);
      } catch {
        setErr('Loglarni yuklab bo‘lmadi');
      }
    };
    load();
    timer.current = setInterval(load, 2000);
    return () => clearInterval(timer.current);
  }, []);

  const rows = [...lines]
    .reverse()
    .filter((l) =>
      filter === 'all' ? true : filter === 'events' ? EVENT_KINDS.has(l.kind) : !EVENT_KINDS.has(l.kind)
    );

  return (
    <div className="space-y-5">
      {learned.length > 0 && (
        <section className="rounded-xl border border-good/40 bg-good/[0.06] px-5 py-3">
          <h2 className="text-sm font-medium text-good">Avtomatik o‘rganildi</h2>
          <p className="mt-1 text-xs text-ink-secondary">
            Ochilish qatori to‘lovga qarab mantiqan aniqlandi (regex kerak bo‘lmadi):
          </p>
          <ul className="mt-2 space-y-1">
            {learned.map((l, i) => (
              <li key={i} className="text-xs text-ink-muted">
                <span className="text-ink-secondary">{l.container}</span> ·{' '}
                <code className="text-good">{l.template}</code>{' '}
                <span className="text-ink-muted/70">
                  ({l.count} marta, {Math.round(l.ratio * 100)}%)
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="rounded-xl border border-line bg-surface">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
          <div>
            <h2 className="text-sm font-medium">Loglar (jonli)</h2>
            <p className="mt-0.5 text-[11px] text-ink-muted">
              Har qatorga ParkPulse qo‘ygan yorliq. <span className="text-warn">OPEN∗</span> —
              avtomatik aniqlangan ochilish.
            </p>
          </div>
          <div className="flex gap-1 rounded-lg border border-line p-0.5">
            {FILTERS.map((f) => (
              <button
                key={f.id}
                onClick={() => setFilter(f.id)}
                className={`rounded-md px-2.5 py-1 text-xs transition-colors ${
                  filter === f.id ? 'bg-white/[0.08] text-ink' : 'text-ink-muted hover:text-ink-secondary'
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>
        </header>

        {err && <p className="px-5 py-3 text-xs text-critical">{err}</p>}

        {rows.length === 0 ? (
          <p className="px-5 py-12 text-center text-sm text-ink-muted">
            Log kutilmoqda… (konteyner tanlanganini tekshiring)
          </p>
        ) : (
          <ul className="max-h-[32rem] divide-y divide-grid overflow-auto font-mono text-xs">
            {rows.map((l, i) => {
              const tag = TAG[l.kind] ?? TAG[''];
              return (
                <li key={i} className="flex items-start gap-2.5 px-4 py-1.5 hover:bg-white/[0.02]">
                  <span className="shrink-0 text-ink-muted/70 [font-variant-numeric:tabular-nums]">
                    {fmtTime(l.time)}
                  </span>
                  <span
                    className={`shrink-0 rounded border px-1.5 text-[10px] leading-5 ${tag.cls}`}
                  >
                    {tag.label}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-ink-secondary" title={l.text}>
                    {l.text}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
