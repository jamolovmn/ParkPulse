'use client';

import { useEffect, useState } from 'react';

type Container = { id: string; name: string; image: string; state: string; watched: boolean };

export default function ContainerPicker() {
  const [list, setList] = useState<Container[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [query, setQuery] = useState('');
  const [err, setErr] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setErr(null);
    try {
      const r = await fetch('/api/containers');
      if (!r.ok) throw new Error(await r.text());
      const d = await r.json();
      const cs: Container[] = d.containers ?? [];
      setList(cs);
      setSelected(new Set(cs.filter((c) => c.watched).map((c) => c.name)));
    } catch {
      setErr('Konteynerlarni yuklab bo‘lmadi — Docker socket ulanmagan bo‘lishi mumkin');
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    load();
  }, []);

  const toggle = async (name: string) => {
    const next = new Set(selected);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    setSelected(next);
    setSaving(true);
    try {
      await fetch('/api/target', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targets: Array.from(next) }),
      });
    } catch {
      /* WS baribir holatni yangilaydi */
    } finally {
      setSaving(false);
    }
  };

  const q = query.trim().toLowerCase();
  const rows = q
    ? list.filter((c) => [c.name, c.image].some((v) => v.toLowerCase().includes(q)))
    : list;

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
        <div>
          <h2 className="text-sm font-medium">
            Kuzatiladigan konteyner{' '}
            {saving && <span className="text-[11px] text-ink-muted">saqlanmoqda…</span>}
          </h2>
          <p className="mt-0.5 text-[11px] text-ink-muted">
            Loglari o‘qiladigan konteyner(lar)ni belgilang. {selected.size} ta tanlangan.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Nom yoki image…"
            className="w-44 rounded-lg border border-line bg-page px-3 py-1.5 text-xs outline-none placeholder:text-ink-muted focus:border-accent"
          />
          <button
            onClick={load}
            title="Yangilash"
            className="rounded-lg border border-line px-2.5 py-1.5 text-xs text-ink-secondary transition-colors hover:bg-white/[0.04]"
          >
            ↻
          </button>
        </div>
      </header>

      {err && <p className="px-5 py-3 text-xs text-critical">{err}</p>}

      {loading ? (
        <p className="px-5 py-10 text-center text-sm text-ink-muted">Yuklanmoqda…</p>
      ) : rows.length === 0 ? (
        <p className="px-5 py-10 text-center text-sm text-ink-muted">Konteyner topilmadi</p>
      ) : (
        <ul className="max-h-80 divide-y divide-grid overflow-auto">
          {rows.map((c) => {
            const on = selected.has(c.name);
            return (
              <li key={c.id}>
                <label className="flex cursor-pointer items-center gap-3 px-5 py-2.5 hover:bg-white/[0.03]">
                  <input
                    type="checkbox"
                    checked={on}
                    onChange={() => toggle(c.name)}
                    className="h-4 w-4 shrink-0 accent-accent"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className={`truncate text-sm ${on ? 'font-medium text-ink' : 'text-ink-secondary'}`}>
                        {c.name}
                      </span>
                      <span
                        className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] ${
                          c.state === 'running' ? 'text-good' : 'text-ink-muted'
                        }`}
                      >
                        {c.state}
                      </span>
                    </div>
                    <span className="truncate text-[11px] text-ink-muted">{c.image}</span>
                  </div>
                </label>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
