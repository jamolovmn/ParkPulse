'use client';

import { Fragment, useState } from 'react';
import { Breakdown, Pass } from '@/lib/useParkPulse';

const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('uz-UZ', { hour12: false });
const fmtMs = (ms: number) => `${ms < 10 ? ms.toFixed(1) : String(Math.round(ms))} ms`;

export default function LiveFeed({ passes }: { passes: Pass[] }) {
  const [open, setOpen] = useState<string | null>(null);

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="border-b border-line px-5 py-3.5">
        <h2 className="text-sm font-medium">Jonli oqim</h2>
        <p className="mt-0.5 text-xs text-ink-muted">
          ANPR’dan to‘lovgacha o‘tgan vaqt — qatorni bosing
        </p>
      </header>

      {passes.length === 0 ? (
        <p className="px-5 py-12 text-center text-sm text-ink-muted">Hodisalar kutilmoqda…</p>
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
                      {/* Pult bilan ochilgan mashinada bu vaqt haydovchining
                          turishi — tizim tezligi emas, o'rtachaga kirmaydi. */}
                      <td
                        className={`px-5 py-2.5 text-right ${
                          p.auto_pay
                            ? 'text-ink-muted'
                            : p.latency_ms > 1500
                              ? 'text-warn'
                              : 'text-ink-secondary'
                        }`}
                        title={p.auto_pay ? "Pult bilan ochilgan — avto to'lov" : undefined}
                      >
                        {p.auto_pay ? "avto to'lov" : fmtMs(p.latency_ms)}
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

function Chain({ breakdown: b }: { breakdown: Breakdown }) {
  const nodes = ['ANPR', 'Gateway', 'DB', 'POS'];
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
