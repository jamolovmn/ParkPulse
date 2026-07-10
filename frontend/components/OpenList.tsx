'use client';

import { useMemo, useState } from 'react';
import { Open, OpenKind, isSuspicious } from '@/lib/useParkPulse';

const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('uz-UZ', { hour12: false });

type Meta = { label: string; icon: string; tone: string; dot: string };

// Holat ranglari doim belgi va yozuv bilan keladi — rangning o'zi ma'no tashimaydi.
const META: Record<OpenKind, Meta> = {
  paid: {
    label: "Dasturda to'landi",
    icon: '✓',
    tone: 'text-good',
    dot: 'bg-good',
  },
  remote: {
    label: "Pult — avto to'lov",
    icon: '⇄',
    tone: 'text-accent',
    dot: 'bg-accent',
  },
  entry: {
    label: 'Kirish',
    icon: '↓',
    tone: 'text-ink-secondary',
    dot: 'bg-ink-muted',
  },
  violation: {
    label: "Qarzdor — to'lovsiz ochildi",
    icon: '▲',
    tone: 'text-warn',
    dot: 'bg-warn',
  },
  ghost: {
    label: "Arvoh — mashinasiz ochildi",
    icon: '✳',
    tone: 'text-critical',
    dot: 'bg-critical',
  },
};

type Filter = 'all' | 'ghost' | 'ok';

const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'Hammasi' },
  { id: 'ghost', label: 'Arvoh ochilishlar' },
  { id: 'ok', label: 'Muammosiz' },
];

export default function OpenList({ opens }: { opens: Open[] }) {
  const [filter, setFilter] = useState<Filter>('all');
  const [open, setOpen] = useState<string | null>(null);

  const rows = useMemo(
    () =>
      opens.filter((o) =>
        filter === 'all' ? true : filter === 'ghost' ? isSuspicious(o.kind) : !isSuspicious(o.kind)
      ),
    [opens, filter]
  );

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
        <h2 className="text-sm font-medium">Shlagbaum ochilishlari</h2>
        <div className="flex gap-1 rounded-lg border border-line p-0.5">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              onClick={() => setFilter(f.id)}
              className={`rounded-md px-2.5 py-1 text-xs transition-colors ${
                filter === f.id
                  ? 'bg-white/[0.08] text-ink'
                  : 'text-ink-muted hover:text-ink-secondary'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
      </header>

      {rows.length === 0 ? (
        <p className="px-5 py-12 text-center text-sm text-ink-muted">
          {filter === 'ghost' ? "Arvoh ochilish yo'q" : 'Hodisalar kutilmoqda…'}
        </p>
      ) : (
        <ul className="divide-y divide-grid">
          {rows.map((o, i) => {
            const key = `${o.open_at}-${o.gate}-${i}`;
            const meta = META[o.kind];
            const suspicious = isSuspicious(o.kind);
            const expanded = open === key;
            const hasContext = (o.context?.length ?? 0) > 0;

            return (
              <li
                key={key}
                onClick={() => hasContext && setOpen(expanded ? null : key)}
                className={`px-5 py-3 ${hasContext ? 'cursor-pointer hover:bg-white/[0.03]' : ''}`}
              >
                <div className="flex items-center gap-2.5 text-sm">
                  <span className={`${meta.tone} shrink-0`} aria-hidden>
                    {meta.icon}
                  </span>
                  <span className={`font-medium ${suspicious ? meta.tone : 'text-ink'}`}>
                    {meta.label}
                  </span>
                  <span className="ml-auto shrink-0 text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
                    {fmtTime(o.open_at)}
                  </span>
                </div>

                <p className="mt-1 text-xs text-ink-secondary">
                  {o.plate ? `${o.plate} · ` : ''}
                  {o.gate || 'darvoza noma’lum'}
                </p>
                <p className="mt-0.5 text-xs text-ink-muted">{o.reason}</p>

                {/* Log faqat shubhali ochilishlar uchun yoziladi */}
                {suspicious && (
                  <p className="mt-1.5 truncate font-mono text-xs text-ink-muted" title={o.raw}>
                    {o.raw}
                  </p>
                )}
                {hasContext && (
                  <p className="mt-1.5 text-xs text-ink-muted">
                    {expanded ? '▾ Kontekst (o‘sha paytdagi loglar)' : '▸ Kontekstni ko‘rish'}
                  </p>
                )}
                {expanded && hasContext && (
                  <pre className="mt-2 max-h-52 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-grid bg-page p-3 text-[11px] leading-relaxed text-ink-muted">
                    {o.context!.join('\n')}
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
