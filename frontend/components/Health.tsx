'use client';

import { SystemHealth } from '@/lib/useParkPulse';

const load = (p: number) => (p < 50 ? 'bg-good' : p < 80 ? 'bg-warn' : 'bg-critical');
const loadText = (p: number) => (p < 50 ? 'text-good' : p < 80 ? 'text-warn' : 'text-critical');

const fmtUptime = (sec: number) => {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return `${d > 0 ? `${d} kun ` : ''}${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
};

/** Meter — foizni 20 ta blok bilan ko'rsatadi (btop uslubi). */
function Meter({ label, percent, hint }: { label: string; percent: number; hint?: string }) {
  const filled = Math.round((Math.min(percent, 100) / 100) * 20);
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-[11px]">
        <span className="text-ink-secondary">{label}</span>
        <span className="text-ink-muted [font-variant-numeric:tabular-nums]">
          {hint ?? `${percent.toFixed(0)}%`}
        </span>
      </div>
      <div className="flex h-2 gap-[2px]">
        {Array.from({ length: 20 }, (_, i) => (
          <div
            key={i}
            className={`flex-1 rounded-[1px] ${i < filled ? load(percent) : 'bg-white/[0.06]'}`}
          />
        ))}
      </div>
    </div>
  );
}

export function HealthPanel({ health }: { health: SystemHealth | null }) {
  if (!health) {
    return (
      <section className="rounded-xl border border-line bg-surface px-5 py-10">
        <p className="text-center text-sm text-ink-muted">Server holati kutilmoqda…</p>
      </section>
    );
  }
  const ram = health.total_ram_mb > 0 ? (health.used_ram_mb / health.total_ram_mb) * 100 : 0;

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 className="text-sm font-medium">Server holati</h2>
        <span className="text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
          {fmtUptime(health.uptime_sec)}
        </span>
      </header>
      <div className="space-y-4 px-5 py-4">
        <div className="grid grid-cols-2 gap-x-5 gap-y-3">
          {health.cores.map((c, i) => (
            <Meter key={i} label={`CPU ${i}`} percent={c} />
          ))}
        </div>
        <Meter
          label="Xotira"
          percent={ram}
          hint={`${(health.used_ram_mb / 1024).toFixed(1)} / ${(health.total_ram_mb / 1024).toFixed(1)} GB`}
        />
      </div>
    </section>
  );
}

export function ContainerTable({ health }: { health: SystemHealth | null }) {
  const rows = [...(health?.containers ?? [])].sort((a, b) => b.cpu_percent - a.cpu_percent);

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="border-b border-line px-5 py-3">
        <h2 className="text-sm font-medium">Konteynerlar yuki</h2>
      </header>
      {rows.length === 0 ? (
        <p className="px-5 py-12 text-center text-sm text-ink-muted">Ma'lumot kutilmoqda…</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm [font-variant-numeric:tabular-nums]">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-ink-muted">
                <th className="px-5 py-2.5 font-medium">Nomi</th>
                <th className="px-5 py-2.5 text-right font-medium">CPU</th>
                <th className="px-5 py-2.5 text-right font-medium">Xotira</th>
                <th className="px-5 py-2.5 text-right font-medium">MB</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c) => (
                <tr key={c.name} className="border-t border-grid hover:bg-white/[0.03]">
                  <td className="px-5 py-2.5 font-medium">{c.name}</td>
                  <td className={`px-5 py-2.5 text-right ${loadText(c.cpu_percent)}`}>
                    {c.cpu_percent.toFixed(1)}%
                  </td>
                  <td className="px-5 py-2.5 text-right text-ink-secondary">
                    {c.ram_percent.toFixed(1)}%
                  </td>
                  <td className="px-5 py-2.5 text-right text-ink-muted">{c.ram_mb.toFixed(0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
