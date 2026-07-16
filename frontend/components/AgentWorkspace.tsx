'use client';

import { useEffect, useRef, useState } from 'react';
import {
  Bot,
  ArrowUp,
  Terminal,
  ScrollText,
  X,
  Settings,
  ShieldAlert,
  KeyRound,
  Square,
  Plus,
} from 'lucide-react';
import AgentSettings, { PULSE_TOKEN } from '@/components/AgentSettings';

// Ikki ustunli ish maydoni (example dizayni bo'yicha):
//   Chap  — suhbat (agent/foydalanuvchi xabarlari, takliflar, kirish maydoni)
//   O'ng  — terminal (bash — buyruq/chiqishi; logs — voqealar oqimi)
// Kalit kiritilmagan bo'lsa avval sozlash sahifasi chiqadi.

type Config = { enabled: boolean; provider: string; model: string; auth: boolean; key_set: boolean };
type ChatItem = { role: 'user' | 'agent'; text: string; time: string };
type TermLine = { kind: 'sys' | 'cmd' | 'out' | 'ok' | 'err'; text: string };
type LogLine = { time: string; text: string; tone: 'good' | 'warn' | 'critical' | 'muted' };
type Confirm = { id: string; command: string; reason: string };

const now = () => new Date().toLocaleTimeString('uz-UZ', { hour: '2-digit', minute: '2-digit' });

const SUGGESTIONS = [
  'Konteynerlar holatini tekshir',
  'Oxirgi konteyner nega qulab tushdi?',
  'Disk va xotira holati',
  'Loglardagi xatolarni ko‘rsat',
];

export default function AgentWorkspace() {
  const [config, setConfig] = useState<Config | null>(null);
  const [loaded, setLoaded] = useState(false);

  // Auth (qurilma tokeni)
  const [pw, setPw] = useState('');
  const [authErr, setAuthErr] = useState('');
  const [token, setToken] = useState<string>('');

  // Ish holati
  const [chat, setChat] = useState<ChatItem[]>([]);
  const [term, setTerm] = useState<TermLine[]>([]);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [status, setStatus] = useState('idle');
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'closed'>('connecting');
  const [input, setInput] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [rightTab, setRightTab] = useState<'bash' | 'logs'>('bash');
  const [termOpen, setTermOpen] = useState(true);
  const [attempt, setAttempt] = useState(0);

  const ws = useRef<WebSocket | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const termEndRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const thinking = status === 'thinking';
  const busy = thinking || status === 'running' || status === 'waiting';

  // Qurilma tokenini o'qish
  useEffect(() => {
    setToken(localStorage.getItem(PULSE_TOKEN) ?? '');
  }, []);

  // Konfiguratsiyani yuklash
  const loadConfig = () =>
    fetch('/api/agent/config')
      .then((r) => r.json())
      .then((c: Config) => setConfig(c))
      .catch(() => setConfig({ enabled: false, provider: '', model: '', auth: false, key_set: false }))
      .finally(() => setLoaded(true));

  useEffect(() => {
    loadConfig();
  }, []);

  // Tayyor bo'lsagina ulanamiz: kalit bor + (auth yo'q yoki token bor)
  const ready = !!config?.enabled && (!config.auth || !!token);

  useEffect(() => {
    if (!ready) return;
    let closed = false;
    const tok = () => token || localStorage.getItem(PULSE_TOKEN) || '';
    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws';
      const q = tok() ? `?token=${encodeURIComponent(tok())}` : '';
      const sock = new WebSocket(`${proto}://${location.host}/api/agent/stream${q}`);
      ws.current = sock;
      setWsState('connecting');
      sock.onopen = () => {
        setWsState('open');
        setTerm((t) =>
          t.length
            ? t
            : [
                { kind: 'sys', text: 'ParkPulse DevOps Agent — sessiya boshlandi' },
                { kind: 'sys', text: `model: ${config?.model || '—'} · provayder: ${config?.provider || '—'}` },
              ]
        );
      };
      sock.onclose = () => {
        setWsState('closed');
        if (!closed) setTimeout(connect, 3000);
      };
      sock.onmessage = (e) => {
        const m = JSON.parse(e.data);
        switch (m.type) {
          case 'status':
            if (m.state === 'reset') {
              // Yangi sessiya — hamma narsani tozalaymiz.
              setChat([]);
              setTerm([]);
              setLogs([]);
              setConfirm(null);
              setStatus('idle');
              break;
            }
            setStatus(m.state);
            if (m.state === 'error' && m.text) {
              setChat((c) => [...c, { role: 'agent', text: `⚠️ ${m.text}`, time: now() }]);
              pushLog(`xato: ${m.text}`, 'critical');
            } else {
              pushLog(`holat: ${m.state}`, 'muted');
            }
            break;
          case 'assistant':
            setChat((c) => [...c, { role: 'agent', text: m.text, time: now() }]);
            pushLog('agent javob berdi', 'good');
            break;
          case 'tool':
            if (m.state === 'running') {
              const cmd = m.name === 'bash' ? m.input || '' : `${m.name} ${m.input || ''}`.trim();
              setTerm((t) => [...t, { kind: 'cmd', text: cmd }]);
              pushLog(`▸ ${cmd}`, 'warn');
              setRightTab('bash');
            } else {
              if (m.output) setTerm((t) => [...t, { kind: 'out', text: String(m.output).replace(/\n+$/, '') }]);
              const ok = (m.exit ?? 0) === 0;
              setTerm((t) => [...t, { kind: ok ? 'ok' : 'err', text: ok ? '✓ tugadi' : `✗ xato (exit ${m.exit})` }]);
              pushLog(ok ? '✓ buyruq tugadi' : `✗ buyruq xato (exit ${m.exit})`, ok ? 'good' : 'critical');
            }
            break;
          case 'confirm':
            setConfirm({ id: m.id, command: m.command, reason: m.reason });
            pushLog(`tasdiq so‘raldi: ${m.command}`, 'warn');
            break;
        }
      };
    };
    const pushLog = (text: string, tone: LogLine['tone']) =>
      setLogs((l) => [...l, { time: now(), text, tone }]);
    connect();
    return () => {
      closed = true;
      ws.current?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, attempt]);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [chat, thinking, confirm]);
  useEffect(() => {
    termEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [term, logs, rightTab]);
  useEffect(() => {
    const ta = taRef.current;
    if (ta) {
      ta.style.height = '0px';
      ta.style.height = Math.min(ta.scrollHeight, 160) + 'px';
    }
  }, [input]);

  const send = (text: string) => {
    const t = text.trim();
    if (!t || ws.current?.readyState !== WebSocket.OPEN) return;
    setChat((c) => [...c, { role: 'user', text: t, time: now() }]);
    ws.current.send(JSON.stringify({ type: 'chat', text: t }));
    setInput('');
  };

  // stop — joriy vazifani to'xtatadi; new — sessiyani tozalaydi (server hal qiladi).
  const control = (type: 'stop' | 'new') => {
    ws.current?.send(JSON.stringify({ type }));
  };

  const decide = (approve: boolean) => {
    if (!confirm) return;
    ws.current?.send(JSON.stringify({ type: 'decision', id: confirm.id, approve }));
    setConfirm(null);
  };

  const login = async () => {
    setAuthErr('');
    try {
      const r = await fetch('/api/agent/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pw }),
      });
      if (!r.ok) return setAuthErr('Parol noto‘g‘ri');
      const d = await r.json();
      localStorage.setItem(PULSE_TOKEN, d.token);
      setToken(d.token);
      setPw('');
      setAttempt((a) => a + 1);
    } catch {
      setAuthErr('Ulanib bo‘lmadi');
    }
  };

  // ---- 1) Yuklanmoqda ----
  if (!loaded) {
    return (
      <div className="flex h-[calc(100vh-9rem)] min-h-[34rem] items-center justify-center rounded-2xl border border-line bg-surface text-sm text-ink-muted">
        Yuklanmoqda…
      </div>
    );
  }

  // ---- 2) Kalit yo'q → avval sozlash sahifasi ----
  if (!config?.enabled) {
    return (
      <div className="mx-auto max-w-2xl space-y-4">
        <div className="rounded-2xl border border-accent/30 bg-accent/[0.06] p-5">
          <div className="flex items-center gap-2">
            <KeyRound className="size-4 text-accent" />
            <h2 className="text-sm font-semibold">Agentni ishga tushirish uchun API kalit kerak</h2>
          </div>
          <p className="mt-1.5 text-xs text-ink-secondary">
            AI agent kalit kiritilmaguncha uxlab turadi. Quyida provayderni tanlab, API kalitini
            kiriting — shundan so‘ng suhbat va terminal ochiladi. Kalit serverda xavfsiz saqlanadi.
          </p>
        </div>
        <AgentSettings onSaved={(s) => s.enabled && loadConfig()} />
      </div>
    );
  }

  // ---- 3) Parol talab qilinadi ----
  if (config.auth && !token) {
    return (
      <section className="mx-auto max-w-md rounded-2xl border border-line bg-surface p-6">
        <div className="flex items-center gap-2">
          <ShieldAlert className="size-4 text-warn" />
          <h2 className="text-sm font-medium">Agent paroli</h2>
        </div>
        <p className="mt-1 text-xs text-ink-muted">
          Agent host’da buyruq bajaradi. Ushbu qurilma uchun bir marta parol kiriting.
        </p>
        <div className="mt-4 flex gap-2">
          <input
            type="password"
            autoFocus
            value={pw}
            onChange={(e) => setPw(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && login()}
            placeholder="Parol"
            className="flex-1 rounded-lg border border-line bg-page px-3 py-2 text-sm outline-none focus:border-accent"
          />
          <button onClick={login} className="rounded-lg bg-accent px-4 py-2 text-sm font-medium text-white hover:opacity-90">
            Kirish
          </button>
        </div>
        {authErr && <p className="mt-2 text-xs text-critical">{authErr}</p>}
      </section>
    );
  }

  // ---- 4) Ish maydoni (ikki ustun) ----
  const empty = chat.length === 0;
  const dot = wsState === 'open' ? (busy ? 'bg-warn' : 'bg-good') : 'bg-critical';
  const stateLabel = wsState === 'open' ? (busy ? 'ishlayapti' : 'tayyor') : wsState === 'connecting' ? 'ulanmoqda' : 'uzilgan';

  return (
    <div className="flex h-[calc(100vh-9rem)] min-h-[34rem] overflow-hidden rounded-2xl border border-line bg-surface">
      {/* ============ CHAP: suhbat ============ */}
      <div className={`flex min-w-0 flex-col ${termOpen ? 'w-full lg:w-[42%] lg:border-r lg:border-line' : 'w-full'}`}>
        <header className="flex shrink-0 items-center gap-2 border-b border-line px-4 py-2.5">
          <span className="flex items-center gap-1.5 rounded-lg bg-white/[0.07] px-2.5 py-1 text-sm font-medium">
            <Bot className="size-3.5 text-accent" />
            Agent
          </span>
          <span className="px-1 text-sm text-ink-muted">Chat</span>
          <span className="ml-auto flex items-center gap-1.5 text-xs text-ink-muted">
            <span className={`size-1.5 rounded-full ${dot}`} />
            {stateLabel}
          </span>
          {busy && (
            <button
              onClick={() => control('stop')}
              title="Joriy vazifani to‘xtatish"
              className="flex items-center gap-1 rounded-lg border border-line px-2 py-1 text-xs text-warn hover:bg-white/[0.04]"
            >
              <Square className="size-3" /> To‘xtat
            </button>
          )}
          <button
            onClick={() => control('new')}
            title="Yangi sessiya — tarixni tozalaydi"
            className="flex items-center gap-1 rounded-lg border border-line px-2 py-1 text-xs text-ink-secondary hover:bg-white/[0.04]"
          >
            <Plus className="size-3.5" /> Yangi
          </button>
          {!termOpen && (
            <button
              onClick={() => setTermOpen(true)}
              title="Terminalni ochish"
              className="rounded-lg border border-line px-2 py-1 text-xs text-ink-secondary hover:bg-white/[0.04]"
            >
              <Terminal className="size-3.5" />
            </button>
          )}
        </header>

        <div className="pp-scroll flex-1 overflow-y-auto">
          <div className="space-y-5 px-4 py-5">
            {chat.map((it, i) =>
              it.role === 'user' ? (
                <div key={i} className="flex justify-end">
                  <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl border border-line bg-page px-4 py-2.5 text-sm leading-relaxed text-ink">
                    {it.text}
                  </div>
                </div>
              ) : (
                <div key={i} className="flex gap-3">
                  <Avatar />
                  <div className="min-w-0">
                    <NameRow time={it.time} />
                    <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-ink-secondary">{it.text}</p>
                  </div>
                </div>
              )
            )}

            {thinking && (
              <div className="flex gap-3">
                <Avatar />
                <span className="flex items-center gap-1.5 pt-1.5 text-xs text-ink-muted">
                  <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '0ms' }} />
                  <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '150ms' }} />
                  <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '300ms' }} />
                  o‘ylayapti…
                </span>
              </div>
            )}

            {confirm && (
              <div className="rounded-xl border border-warn/40 bg-warn/[0.08] p-3">
                <p className="flex items-center gap-1.5 text-xs font-medium text-warn">
                  <ShieldAlert className="size-3.5" /> Tasdiq kerak — xavfli buyruq
                </p>
                <pre className="mt-1.5 overflow-x-auto whitespace-pre-wrap rounded-lg bg-page p-2 font-mono text-xs text-ink">
                  {confirm.command}
                </pre>
                <p className="mt-1 text-xs text-ink-muted">{confirm.reason}</p>
                <div className="mt-2.5 flex gap-2">
                  <button onClick={() => decide(true)} className="rounded-lg bg-critical px-3 py-1.5 text-xs font-medium text-white hover:opacity-90">
                    Ha, bajar
                  </button>
                  <button onClick={() => decide(false)} className="rounded-lg border border-line px-3 py-1.5 text-xs text-ink-secondary hover:bg-white/[0.04]">
                    Yo‘q
                  </button>
                </div>
              </div>
            )}
            <div ref={chatEndRef} />
          </div>

          {/* Takliflar (bo'sh holatda) */}
          {empty && (
            <div className="flex flex-wrap gap-2 px-4 pb-2">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => send(s)}
                  className="rounded-lg border border-line px-3 py-1.5 text-xs text-ink-secondary transition-colors hover:border-accent/40 hover:text-ink"
                >
                  {s}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Kirish maydoni (composer) */}
        <div className="shrink-0 px-3 pb-3">
          <div className="rounded-2xl border border-line bg-page px-3 py-2.5 focus-within:border-accent/60">
            <textarea
              ref={taRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  send(input);
                }
              }}
              rows={1}
              disabled={wsState !== 'open'}
              placeholder={wsState === 'open' ? 'Rejalashtir, tekshir yoki tuzat — agent terminalga ega.' : 'Ulanmoqda…'}
              className="max-h-40 w-full resize-none bg-transparent text-sm text-ink placeholder:text-ink-muted focus:outline-none disabled:opacity-50"
            />
            <div className="mt-1.5 flex items-center gap-3">
              <button
                onClick={() => setSettingsOpen(true)}
                className="text-ink-muted transition-colors hover:text-ink"
                title="Provayder / modelni o‘zgartirish"
                aria-label="Sozlamalar"
              >
                <Settings className="size-4" />
              </button>
              <button
                onClick={() => send(input)}
                disabled={wsState !== 'open' || !input.trim()}
                className="ml-auto flex size-8 items-center justify-center rounded-xl bg-accent text-white transition-opacity hover:opacity-90 disabled:opacity-40"
                aria-label="Yuborish"
              >
                <ArrowUp className="size-4" />
              </button>
            </div>
          </div>
          {wsState === 'closed' && <p className="mt-1.5 text-center text-[11px] text-ink-muted">Ulanish uzildi — qayta urinmoqda…</p>}
        </div>
      </div>

      {/* ============ O'NG: terminal ============ */}
      {termOpen && (
        <div className="hidden min-w-0 flex-1 flex-col bg-term-bg lg:flex">
          <header className="flex shrink-0 items-center gap-1 border-b border-line px-3 py-2">
            <button
              onClick={() => setRightTab('bash')}
              className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs ${
                rightTab === 'bash' ? 'bg-white/[0.07] text-ink' : 'text-ink-muted hover:text-ink-secondary'
              }`}
            >
              <Terminal className="size-3.5" /> bash
            </button>
            <button
              onClick={() => setRightTab('logs')}
              className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs ${
                rightTab === 'logs' ? 'bg-white/[0.07] text-ink' : 'text-ink-muted hover:text-ink-secondary'
              }`}
            >
              <ScrollText className="size-3.5" /> logs
            </button>
            <span className="ml-auto flex items-center gap-1.5 text-xs text-ink-muted">
              <span className={`size-1.5 rounded-full ${dot}`} />
              {stateLabel}
            </span>
            <button
              onClick={() => setTermOpen(false)}
              title="Terminalni yopish"
              className="ml-1 rounded p-1 text-ink-muted hover:text-ink"
              aria-label="Terminalni yopish"
            >
              <X className="size-4" />
            </button>
          </header>

          <div className="pp-scroll flex-1 overflow-y-auto px-4 py-3 font-mono text-[12.5px] leading-relaxed">
            {rightTab === 'bash' ? (
              <>
                {term.map((l, i) => (
                  <TermRow key={i} line={l} />
                ))}
                <div className="flex items-center gap-1 text-term-green">
                  <span>$</span>
                  <span className="pp-cursor inline-block h-3.5 w-2 bg-term-gray/70" />
                </div>
                <div ref={termEndRef} />
              </>
            ) : (
              <>
                {logs.length === 0 && <p className="text-term-gray">Voqealar hali yo‘q.</p>}
                {logs.map((l, i) => (
                  <div key={i} className="flex gap-2">
                    <span className="text-term-gray">{l.time}</span>
                    <span className={toneClass(l.tone)}>{l.text}</span>
                  </div>
                ))}
                <div ref={termEndRef} />
              </>
            )}
          </div>
        </div>
      )}

      {/* Sozlamalar oynasi */}
      {settingsOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-black/60 p-4 sm:p-8">
          <div className="w-full max-w-2xl">
            <div className="mb-3 flex justify-end">
              <button
                onClick={() => {
                  setSettingsOpen(false);
                  loadConfig();
                }}
                className="rounded-lg border border-line bg-surface p-1.5 text-ink-muted hover:text-ink"
                aria-label="Yopish"
              >
                <X className="size-4" />
              </button>
            </div>
            <AgentSettings onSaved={() => loadConfig()} />
          </div>
        </div>
      )}
    </div>
  );
}

function Avatar() {
  return (
    <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-lg bg-white/[0.06] ring-1 ring-white/10">
      <Bot className="size-3.5 text-ink-secondary" />
    </span>
  );
}

function NameRow({ time }: { time?: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm font-medium text-ink">Agent</span>
      {time && <span className="text-[11px] text-ink-muted [font-variant-numeric:tabular-nums]">{time}</span>}
    </div>
  );
}

function TermRow({ line }: { line: TermLine }) {
  if (line.kind === 'sys') return <p className="text-term-gray">{line.text}</p>;
  if (line.kind === 'cmd')
    return (
      <p className="mt-1 whitespace-pre-wrap break-words">
        <span className="text-term-green">$ </span>
        <span className="text-ink">{line.text}</span>
      </p>
    );
  if (line.kind === 'ok') return <p className="text-term-green">{line.text}</p>;
  if (line.kind === 'err') return <p className="text-term-red">{line.text}</p>;
  return <pre className="whitespace-pre-wrap break-words text-term-gray">{line.text}</pre>;
}

function toneClass(t: LogLine['tone']) {
  return t === 'good'
    ? 'text-term-green'
    : t === 'warn'
    ? 'text-term-yellow'
    : t === 'critical'
    ? 'text-term-red'
    : 'text-term-gray';
}
