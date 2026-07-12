'use client';

import { SnmpHost, SnmpIface } from '@/lib/useParkPulse';

const fmtMbps = (v: number) => {
  if (v <= 0) return '0';
  if (v < 1) return v.toFixed(2);
  if (v < 10) return v.toFixed(1);
  return String(Math.round(v));
};

// Interfeys bandligi (foiz): joriy trafik / liniya tezligi. Tezlik noma'lum
// bo'lsa bar chizilmaydi (noto'g'ri foiz ko'rsatmaslik uchun).
function util(mbps: number, speed?: number): number | null {
  if (!speed || speed <= 0) return null;
  return Math.min((mbps / speed) * 100, 100);
}

function Bar({ pct, tone }: { pct: number | null; tone: string }) {
  if (pct === null) return <span className="text-ink-muted/40">—</span>;
  return (
    <span className="inline-flex h-1.5 w-16 overflow-hidden rounded-full bg-white/[0.06] align-middle">
      <span className={`h-full rounded-full ${tone}`} style={{ width: `${Math.max(pct, 2)}%` }} />
    </span>
  );
}

function IfaceRow({ i }: { i: SnmpIface }) {
  return (
    <tr className="border-t border-grid hover:bg-white/[0.03]">
      <td className="px-4 py-2">
        <span className="flex items-center gap-2">
          <span
            className={`h-1.5 w-1.5 shrink-0 rounded-full ${i.up ? 'bg-good' : 'bg-ink-muted'}`}
            title={i.up ? 'up' : 'down'}
          />
          <span className={i.up ? '' : 'text-ink-muted'}>{i.name}</span>
        </span>
      </td>
      <td className="px-4 py-2 text-right text-accent [font-variant-numeric:tabular-nums]">
        {fmtMbps(i.in_mbps)}
      </td>
      <td className="hidden px-2 py-2 sm:table-cell">
        <Bar pct={util(i.in_mbps, i.speed_mbps)} tone="bg-accent" />
      </td>
      <td className="px-4 py-2 text-right text-good [font-variant-numeric:tabular-nums]">
        {fmtMbps(i.out_mbps)}
      </td>
      <td className="hidden px-2 py-2 sm:table-cell">
        <Bar pct={util(i.out_mbps, i.speed_mbps)} tone="bg-good" />
      </td>
      <td className="px-4 py-2 text-right text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
        {i.speed_mbps ? (i.speed_mbps >= 1000 ? `${i.speed_mbps / 1000}G` : `${i.speed_mbps}M`) : '—'}
      </td>
    </tr>
  );
}

function HostCard({ host }: { host: SnmpHost }) {
  // Faol interfeyslar (up yoki trafik bor) tepada; qolgani pastda.
  const ifaces = [...host.ifaces].sort((a, b) => {
    const av = a.up ? 1 : 0;
    const bv = b.up ? 1 : 0;
    if (av !== bv) return bv - av;
    return b.in_mbps + b.out_mbps - (a.in_mbps + a.out_mbps);
  });

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-line px-5 py-3">
        <span className={`h-2 w-2 rounded-full ${host.up ? 'bg-good' : 'bg-critical'}`} />
        <h3 className="text-sm font-medium">{host.name}</h3>
        <span className="text-xs text-ink-muted">{host.ip}</span>
        {host.uptime && (
          <span className="ml-auto text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
            ↑ {host.uptime}
          </span>
        )}
      </header>

      {host.err ? (
        <p className="px-5 py-4 text-sm text-critical">{host.err}</p>
      ) : ifaces.length === 0 ? (
        <p className="px-5 py-6 text-center text-sm text-ink-muted">Interfeys topilmadi</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-ink-muted">
                <th className="px-4 py-2 font-medium">Interfeys</th>
                <th className="px-4 py-2 text-right font-medium">↓ Mbps</th>
                <th className="hidden px-2 py-2 sm:table-cell" />
                <th className="px-4 py-2 text-right font-medium">↑ Mbps</th>
                <th className="hidden px-2 py-2 sm:table-cell" />
                <th className="px-4 py-2 text-right font-medium">Liniya</th>
              </tr>
            </thead>
            <tbody>
              {ifaces.map((i) => (
                <IfaceRow key={i.index} i={i} />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {host.descr && (
        <p className="truncate border-t border-grid px-5 py-2 text-[11px] text-ink-muted" title={host.descr}>
          {host.descr}
        </p>
      )}
    </section>
  );
}

export default function SnmpPanel({ hosts }: { hosts: SnmpHost[] }) {
  if (hosts.length === 0) {
    return (
      <section className="rounded-xl border border-line bg-surface px-5 py-10">
        <p className="text-center text-sm text-ink-muted">
          SNMP qurilmasi sozlanmagan. <code className="text-ink-secondary">SNMP_TARGETS</code> bering
          (masalan <code className="text-ink-secondary">Core=192.168.1.1@public</code>).
        </p>
      </section>
    );
  }
  return (
    <div className="space-y-6">
      {hosts.map((h) => (
        <HostCard key={h.ip} host={h} />
      ))}
    </div>
  );
}
