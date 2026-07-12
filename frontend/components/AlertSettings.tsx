'use client';

import { useEffect, useState } from 'react';

type Config = { telegram_token: string; telegram_chat: string; webhook: string };
type Status = { enabled: boolean; sinks: string[] };

const empty: Config = { telegram_token: '', telegram_chat: '', webhook: '' };

const field =
  'w-full rounded-lg border border-line bg-page px-3 py-2 text-sm text-ink placeholder:text-ink-muted focus:border-accent focus:outline-none';

export default function AlertSettings() {
  const [cfg, setCfg] = useState<Config>(empty);
  const [status, setStatus] = useState<Status>({ enabled: false, sinks: [] });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    fetch('/api/alerts')
      .then((r) => r.json())
      .then((d) => {
        setCfg({ ...empty, ...(d.config ?? {}) });
        setStatus({ enabled: !!d.enabled, sinks: d.sinks ?? [] });
      })
      .catch(() => setMsg({ ok: false, text: 'Sozlamani yuklab bo‘lmadi' }))
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await fetch('/api/alerts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cfg),
      });
      if (!r.ok) throw new Error(await r.text());
      const d = await r.json();
      setStatus({ enabled: !!d.enabled, sinks: d.sinks ?? [] });
      setMsg({ ok: true, text: 'Saqlandi' });
    } catch (e) {
      setMsg({ ok: false, text: `Saqlashda xato: ${e instanceof Error ? e.message : e}` });
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await fetch('/api/alerts/test', { method: 'POST' });
      const d = await r.json();
      setMsg(
        d.ok
          ? { ok: true, text: 'Sinov xabari yuborildi — Telegram/webhook’ni tekshiring' }
          : { ok: false, text: `Sinov muvaffaqiyatsiz: ${d.error}` }
      );
    } catch (e) {
      setMsg({ ok: false, text: `Sinov xatosi: ${e instanceof Error ? e.message : e}` });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 className="text-sm font-medium">Ogohlantirish</h2>
        <span
          className={`flex items-center gap-1.5 text-xs ${
            status.enabled ? 'text-good' : 'text-ink-muted'
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${status.enabled ? 'bg-good' : 'bg-ink-muted'}`}
          />
          {status.enabled ? `Yoqilgan · ${status.sinks.join(', ')}` : 'O‘chirilgan'}
        </span>
      </header>

      <div className="space-y-4 px-5 py-4">
        <p className="text-xs text-ink-muted">
          Qurilma uzilsa, arvoh ochilish bo‘lsa yoki SNMP porti uzilsa — Telegram va/yoki webhook’ga
          xabar. Faqat holat o‘zgarganda yuboriladi.
        </p>

        <fieldset disabled={loading || busy} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">Telegram bot token</span>
              <input
                type="password"
                className={field}
                placeholder="123456:ABC-DEF…"
                value={cfg.telegram_token}
                onChange={(e) => setCfg({ ...cfg, telegram_token: e.target.value })}
                autoComplete="off"
              />
              <span className="block text-[11px] text-ink-muted">@BotFather bergan token</span>
            </label>
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">Telegram chat ID</span>
              <input
                type="text"
                className={field}
                placeholder="-1001234567890"
                value={cfg.telegram_chat}
                onChange={(e) => setCfg({ ...cfg, telegram_chat: e.target.value })}
                autoComplete="off"
              />
              <span className="block text-[11px] text-ink-muted">
                kanal/guruh yoki shaxsiy chat ID
              </span>
            </label>
          </div>

          <label className="block space-y-1.5">
            <span className="text-xs font-medium text-ink-secondary">Webhook URL (ixtiyoriy)</span>
            <input
              type="text"
              className={field}
              placeholder="https://example.com/hook"
              value={cfg.webhook}
              onChange={(e) => setCfg({ ...cfg, webhook: e.target.value })}
              autoComplete="off"
            />
          </label>

          <div className="flex flex-wrap items-center gap-3">
            <button
              onClick={save}
              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
            >
              Saqlash
            </button>
            <button
              onClick={test}
              disabled={!status.enabled}
              className="rounded-lg border border-line px-4 py-2 text-sm text-ink-secondary transition-colors hover:bg-white/[0.04] disabled:opacity-40"
            >
              Sinov yuborish
            </button>
            {msg && (
              <span className={`text-xs ${msg.ok ? 'text-good' : 'text-critical'}`}>{msg.text}</span>
            )}
          </div>
        </fieldset>
      </div>
    </section>
  );
}
