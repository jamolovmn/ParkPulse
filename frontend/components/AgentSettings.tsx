'use client';

import { useEffect, useState } from 'react';

type Provider = 'anthropic' | 'openai' | 'openrouter' | 'nvidia' | 'local';
type Status = {
  enabled: boolean;
  provider: Provider;
  model: string;
  base_url: string;
  key_set: boolean;
  key_hint?: string;
  auth: boolean;
};

const PROVIDERS: { id: Provider; label: string; modelHint: string; keyLabel: string }[] = [
  { id: 'anthropic', label: 'Anthropic (Claude)', modelHint: 'claude-opus-4-8', keyLabel: 'sk-ant-…' },
  { id: 'openrouter', label: 'OpenRouter (ko‘p model)', modelHint: 'anthropic/claude-opus-4-8', keyLabel: 'sk-or-…' },
  { id: 'openai', label: 'OpenAI', modelHint: 'gpt-4o', keyLabel: 'sk-…' },
  { id: 'nvidia', label: 'NVIDIA', modelHint: 'meta/llama-3.1-70b-instruct', keyLabel: 'nvapi-…' },
  { id: 'local', label: 'Local / boshqa (OpenAI-mos)', modelHint: 'llama3.1', keyLabel: 'ixtiyoriy' },
];

export const PULSE_TOKEN = 'pulse_token'; // localStorage kaliti (qurilma tokeni)

const field =
  'w-full rounded-lg border border-line bg-page px-3 py-2 text-sm text-ink placeholder:text-ink-muted focus:border-accent focus:outline-none';

export default function AgentSettings() {
  const [status, setStatus] = useState<Status | null>(null);
  const [provider, setProvider] = useState<Provider>('anthropic');
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState('');
  const [baseUrl, setBaseUrl] = useState('');
  const [password, setPassword] = useState('');
  const [models, setModels] = useState<string[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const meta = PROVIDERS.find((p) => p.id === provider)!;

  const fetchModels = async () => {
    setLoadingModels(true);
    try {
      const token = localStorage.getItem(PULSE_TOKEN) ?? '';
      const r = await fetch('/api/agent/models', { headers: token ? { 'X-Pulse-Token': token } : {} });
      const d = await r.json();
      setModels(Array.isArray(d.models) ? d.models : []);
    } catch {
      setModels([]);
    } finally {
      setLoadingModels(false);
    }
  };

  useEffect(() => {
    fetch('/api/agent/config')
      .then((r) => r.json())
      .then((s: Status) => {
        setStatus(s);
        setProvider(s.provider || 'anthropic');
        setModel(s.model || '');
        setBaseUrl(s.base_url || '');
        if (s.key_set) fetchModels(); // kalit bor bo'lsa modellarni avto tortadi
      })
      .catch(() => setMsg({ ok: false, text: 'Holatni yuklab bo‘lmadi' }));
  }, []);

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const token = localStorage.getItem(PULSE_TOKEN) ?? '';
      const r = await fetch('/api/agent/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(token ? { 'X-Pulse-Token': token } : {}) },
        // apiKey/password bo'sh bo'lsa yubormaymiz — server eskisini saqlaydi
        body: JSON.stringify({
          provider,
          model,
          base_url: baseUrl,
          ...(apiKey ? { api_key: apiKey } : {}),
          ...(password ? { password } : {}),
        }),
      });
      if (r.status === 401) throw new Error('parol kerak — Agent bo‘limida kiring');
      if (!r.ok) throw new Error(await r.text());
      setStatus(await r.json());
      setApiKey('');
      setPassword('');
      setMsg({ ok: true, text: 'Saqlandi' });
    } catch (e) {
      setMsg({ ok: false, text: `Xato: ${e instanceof Error ? e.message : e}` });
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const d = await (await fetch('/api/agent/test', { method: 'POST' })).json();
      setMsg(d.ok ? { ok: true, text: 'Kalit ishlayapti ✓' } : { ok: false, text: `Tekshiruv: ${d.error}` });
    } catch (e) {
      setMsg({ ok: false, text: `Xato: ${e instanceof Error ? e.message : e}` });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="rounded-xl border border-line bg-surface">
      <header className="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 className="text-sm font-medium">AI agent — provayder sozlamasi</h2>
        <span className={`flex items-center gap-1.5 text-xs ${status?.enabled ? 'text-good' : 'text-ink-muted'}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${status?.enabled ? 'bg-good' : 'bg-ink-muted'}`} />
          {status?.enabled ? 'Faol' : 'Uxlab turibdi'}
        </span>
      </header>

      <div className="space-y-4 px-5 py-4">
        <p className="text-xs text-ink-muted">
          Agent kalit kiritilmaguncha ishlamaydi. Kalit serverda xavfsiz saqlanadi va UI’ga
          hech qachon ochiq qaytarilmaydi.
        </p>

        <fieldset disabled={busy} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">Provayder</span>
              <select className={field} value={provider} onChange={(e) => setProvider(e.target.value as Provider)}>
                {PROVIDERS.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">Model</span>
              <div className="flex gap-2">
                {models.length > 0 ? (
                  <select className={field} value={model} onChange={(e) => setModel(e.target.value)}>
                    <option value="">— tanlang —</option>
                    {model && !models.includes(model) && <option value={model}>{model} (joriy)</option>}
                    {models.map((m) => (
                      <option key={m} value={m}>
                        {m}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input className={field} placeholder={meta.modelHint} value={model} onChange={(e) => setModel(e.target.value)} />
                )}
                <button
                  type="button"
                  onClick={fetchModels}
                  title="Modellarni provayderdan tortish"
                  className="shrink-0 rounded-lg border border-line px-2.5 text-sm text-ink-secondary transition-colors hover:bg-white/[0.04]"
                >
                  {loadingModels ? '…' : '↻'}
                </button>
              </div>
              <span className="block text-[11px] text-ink-muted">
                {models.length ? `${models.length} ta model — ro‘yxatdan tanlang` : 'Kalitni saqlab ↻ bosing — avto ro‘yxat'}
              </span>
            </label>
          </div>

          <label className="block space-y-1.5">
            <span className="text-xs font-medium text-ink-secondary">
              API kalit{' '}
              {status?.key_set && <span className="text-ink-muted">(o‘rnatilgan: {status.key_hint} — o‘zgartirish uchun yangisini kiriting)</span>}
            </span>
            <input
              type="password"
              className={field}
              placeholder={meta.keyLabel}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              autoComplete="off"
            />
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">Baza URL (ixtiyoriy)</span>
              <input
                className={field}
                placeholder={status?.base_url || 'provayder standarti'}
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
              />
              <span className="block text-[11px] text-ink-muted">
                Bo‘sh — standart. Har qanday OpenAI-mos provayder (MiniMax, Groq, DeepSeek…) shu yerdan.
              </span>
            </label>
            <label className="space-y-1.5">
              <span className="text-xs font-medium text-ink-secondary">
                Agent paroli {status?.auth && <span className="text-good">(o‘rnatilgan)</span>}
              </span>
              <input
                type="password"
                className={field}
                placeholder={status?.auth ? 'o‘zgartirish uchun yangisini' : 'agentni himoyalash (ixtiyoriy)'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
              />
              <span className="block text-[11px] text-ink-muted">
                O‘rnatilsa, Agent’ga har yangi qurilma parol so‘raydi.
              </span>
            </label>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <button
              onClick={save}
              className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
            >
              Saqlash
            </button>
            <button
              onClick={test}
              disabled={!status?.enabled}
              className="rounded-lg border border-line px-4 py-2 text-sm text-ink-secondary transition-colors hover:bg-white/[0.04] disabled:opacity-40"
            >
              Kalitni tekshirish
            </button>
            {msg && <span className={`text-xs ${msg.ok ? 'text-good' : 'text-critical'}`}>{msg.text}</span>}
          </div>
        </fieldset>
      </div>
    </section>
  );
}
