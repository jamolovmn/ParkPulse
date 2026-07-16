'use client';

import { useMemo, useRef, useState } from 'react';
import { Device } from '@/lib/useParkPulse';
import Sparkline from '@/components/Sparkline';

const fmtMs = (ms: number) => `${ms < 10 ? ms.toFixed(1) : String(Math.round(ms))} ms`;

// Jitter/loss rangi: past = yaxshi, yuqori = yomon.
const jitterTone = (j: number) => (j < 5 ? 'text-ink-muted' : j < 20 ? 'text-warn' : 'text-critical');
const lossTone = (l: number) => (l === 0 ? 'text-ink-muted' : l < 10 ? 'text-warn' : 'text-critical');

/**
 * Qurilmalar bo'limi. Ilgari subnet skaner tugmasi dashboard yon panelida
 * turardi — bu yerda u o'z bo'limida: filtr, qidiruv va skaner bir joyda.
 */
export default function Devices({ devices }: { devices: Device[] }) {
  const [query, setQuery] = useState('');
  const [subnet, setSubnet] = useState('');
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [found, setFound] = useState<number | null>(null);

  // Kuzatuvni yoqish/o'chirish — faqat ★ qurilmalar uzilganda alert beradi.
  // Server tanlovni saqlaydi va yangilangan ro'yxatni WS orqali darhol qaytaradi.
  const toggleWatch = (d: Device) => {
    fetch('/api/devices/watch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip: d.ip, watched: !d.watched }),
    }).catch(() => {});
  };

  // Qo'lda nom berish. Bo'sh nom — avtomatik nomga qaytaradi (server hal qiladi).
  const [editIp, setEditIp] = useState<string | null>(null);
  const [nameVal, setNameVal] = useState('');
  const skipBlur = useRef(false); // Escape'da saqlamaslik uchun

  const startEdit = (d: Device) => {
    setEditIp(d.ip);
    // Nomi yo'q (avto-nom = IP) bo'lsa bo'sh boshlaymiz — IP tahrir emas, yangi nom.
    setNameVal(d.name === d.ip ? '' : d.name);
  };
  const commit = (ip: string) => {
    if (skipBlur.current) {
      skipBlur.current = false;
      setEditIp(null);
      return;
    }
    setEditIp(null);
    fetch('/api/devices/name', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip, name: nameVal.trim() }),
    }).catch(() => {});
  };

  const scan = async () => {
    setScanning(true);
    setError(null);
    setFound(null);
    try {
      const r = await fetch('/api/scan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ subnet: subnet.trim() }),
      });
      if (!r.ok) {
        setError((await r.text()).trim() || 'Skaner xatosi');
        return;
      }
      const data = await r.json();
      setFound(data.found ?? 0);
    } catch {
      setError('Serverga ulanib bo‘lmadi');
    } finally {
      setScanning(false);
    }
  };

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? devices.filter((d) =>
          [d.name, d.ip, d.type, d.vendor].some((v) => v?.toLowerCase().includes(q))
        )
      : devices;
    // ★ belgilanganlar (kuzatiladigan) eng tepada; keyin uzilganlar (muammo
    // ko'zga birinchi tashlansin); oxirida IP bo'yicha.
    return [...list].sort(
      (a, b) =>
        Number(b.watched) - Number(a.watched) ||
        Number(a.alive) - Number(b.alive) ||
        a.ip.localeCompare(b.ip)
    );
  }, [devices, query]);

  const online = devices.filter((d) => d.alive).length;

  return (
    <div className="space-y-5">
      <section className="rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-medium">Tarmoqni skanerlash</h2>
        <p className="mt-1 text-xs text-ink-muted">
          Subnetni bo‘sh qoldirsangiz server o‘z tarmog‘ini tekshiradi.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <input
            value={subnet}
            onChange={(e) => setSubnet(e.target.value)}
            placeholder="192.168.1.0/24"
            className="min-w-[200px] flex-1 rounded-lg border border-line bg-page px-3 py-2 text-sm outline-none placeholder:text-ink-muted focus:border-accent"
          />
          <button
            onClick={scan}
            disabled={scanning}
            className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            {scanning ? 'Qidirilmoqda…' : 'Qidirish'}
          </button>
        </div>
        {error && <p className="mt-2 text-xs text-critical">{error}</p>}
        {found !== null && (
          <p className="mt-2 text-xs text-good">{found} ta qurilma topildi</p>
        )}
      </section>

      <section className="rounded-xl border border-line bg-surface">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
          <div>
            <h2 className="text-sm font-medium">
              Qurilmalar{' '}
              <span className="text-ink-muted [font-variant-numeric:tabular-nums]">
                {online}/{devices.length} onlayn
              </span>
            </h2>
            <p className="mt-0.5 text-[11px] text-ink-muted">
              <span className="text-warn">★</span> = uzilganda xabar beradi ·{' '}
              <span className="text-ink-muted/50">☆</span> = e’tiborsiz
            </p>
          </div>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Nom, IP yoki turi…"
            className="w-56 rounded-lg border border-line bg-page px-3 py-1.5 text-xs outline-none placeholder:text-ink-muted focus:border-accent"
          />
        </header>

        {rows.length === 0 ? (
          <p className="px-5 py-12 text-center text-sm text-ink-muted">
            {devices.length === 0 ? "Qurilma yo'q — tarmoqni skanerlang" : 'Topilmadi'}
          </p>
        ) : (
          <ul className="divide-y divide-grid">
            {rows.map((d) => {
              const hasQuality = (d.samples?.length ?? 0) > 1;
              // Avto-topilgan qurilmada nom = IP. Shunda IP ni ikki marta
              // ko'rsatmaymiz va pastki qatorda faqat portlar qoladi.
              const hasName = d.name !== d.ip;
              const sub = [hasName ? d.ip : '', d.ports?.length ? d.ports.join(', ') : '']
                .filter(Boolean)
                .join(' · ');
              return (
                <li key={d.ip} className="px-5 py-3 text-sm">
                  <div className="flex items-center gap-3">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${d.alive ? 'bg-good' : 'bg-critical'}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        {editIp === d.ip ? (
                          <input
                            autoFocus
                            value={nameVal}
                            onChange={(e) => setNameVal(e.target.value)}
                            onBlur={() => commit(d.ip)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') e.currentTarget.blur();
                              if (e.key === 'Escape') {
                                skipBlur.current = true;
                                e.currentTarget.blur();
                              }
                            }}
                            placeholder="Nom (Relay, Kamera, LED…)"
                            className="w-48 rounded border border-accent bg-page px-2 py-0.5 text-sm outline-none"
                          />
                        ) : (
                          <>
                            <span
                              className={`truncate font-medium ${
                                hasName ? '' : 'text-ink-secondary [font-variant-numeric:tabular-nums]'
                              }`}
                            >
                              {hasName ? d.name : d.ip}
                            </span>
                            <button
                              onClick={() => startEdit(d)}
                              title={hasName ? 'Nomni tahrirlash' : 'Nom berish'}
                              aria-label="Nom berish"
                              className="text-xs text-ink-muted/50 transition-colors hover:text-ink-muted"
                            >
                              ✎
                            </button>
                            {d.type && (
                              <span className="rounded border border-line px-1.5 py-0.5 text-[10px] text-ink-muted">
                                {d.type}
                              </span>
                            )}
                            {d.vendor && (
                              <span className="text-[10px] text-ink-muted">{d.vendor}</span>
                            )}
                          </>
                        )}
                      </div>
                      {sub && (
                        <span className="text-xs text-ink-muted [font-variant-numeric:tabular-nums]">
                          {sub}
                        </span>
                      )}
                    </div>
                    {hasQuality && <Sparkline data={d.samples!} />}
                    <span
                      className={`shrink-0 text-right text-xs [font-variant-numeric:tabular-nums] ${
                        d.alive ? 'text-ink-secondary' : 'text-critical'
                      }`}
                    >
                      {d.alive ? fmtMs(d.rtt_ms) : 'uzilgan'}
                    </span>
                    <button
                      onClick={() => toggleWatch(d)}
                      title={
                        d.watched
                          ? 'Kuzatilmoqda — uzilsa xabar keladi. O‘chirish uchun bosing'
                          : 'Kuzatilmayapti — xabar yo‘q. Yoqish uchun bosing'
                      }
                      aria-label="Kuzatishni almashtirish"
                      className={`shrink-0 text-base leading-none transition-colors ${
                        d.watched ? 'text-warn' : 'text-ink-muted/40 hover:text-ink-muted'
                      }`}
                    >
                      {d.watched ? '★' : '☆'}
                    </button>
                  </div>

                  {/* Ping sifati: jitter, yo'qotish, uptime, min/avg/max */}
                  {hasQuality && (
                    <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 pl-5 text-[11px] text-ink-muted [font-variant-numeric:tabular-nums]">
                      <Stat label="jitter" value={`${d.jitter_ms ?? 0} ms`} tone={jitterTone(d.jitter_ms ?? 0)} />
                      <Stat label="yo'qotish" value={`${d.loss_pct ?? 0}%`} tone={lossTone(d.loss_pct ?? 0)} />
                      <Stat label="uptime" value={`${d.uptime_pct ?? 0}%`} />
                      <Stat label="min/o'rt/max" value={`${d.min_ms ?? 0} / ${d.avg_ms ?? 0} / ${d.max_ms ?? 0}`} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <span className="flex items-center gap-1">
      <span className="text-ink-muted/70">{label}:</span>
      <span className={tone ?? 'text-ink-secondary'}>{value}</span>
    </span>
  );
}
