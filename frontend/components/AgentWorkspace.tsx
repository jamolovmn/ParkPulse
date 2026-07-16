'use client';

import { useEffect, useRef, useState } from 'react';
import {
  Bot,
  Settings as SettingsIcon,
  ArrowUp,
  Terminal,
  FileText,
  Boxes,
  Wrench,
  Loader2,
  Check,
  X,
  ChevronDown,
  ShieldAlert,
  Cpu,
} from 'lucide-react';
import AgentSettings, { PULSE_TOKEN } from '@/components/AgentSettings';

// Bitta timeline: foydalanuvchi/agent xabarlari + agent amallari (tool qadamlari)
// bir oqimda ketma-ket. Alohida terminal yo'q — toza chat.

type UserItem = { kind: 'user'; text: string; time: string };
type AgentItem = { kind: 'agent'; text: string; time: string };
type ToolItem = {
  kind: 'tool';
  id: string;
  name: string;
  input: string;
  status: 'running' | 'done';
  output?: string;
  exit?: number;
};
type Item = UserItem | AgentItem | ToolItem;
type Confirm = { id: string; command: string; reason: string };

const now = () => new Date().toLocaleTimeString('uz-UZ', { hour: '2-digit', minute: '2-digit' });

const SUGGESTIONS = [
  'Nega oxirgi konteyner qulab tushdi?',
  'Konteynerlarni ko‘rsat',
  'Disk va xotira holati',
];

export default function AgentWorkspace() {
  const [items, setItems] = useState<Item[]>([]);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [status, setStatus] = useState('idle');
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'closed'>('connecting');
  const [input, setInput] = useState('');
  const [model, setModel] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false);

  const [gate, setGate] = useState(false);
  const [pw, setPw] = useState('');
  const [authErr, setAuthErr] = useState('');
  const [attempt, setAttempt] = useState(0);

  const ws = useRef<WebSocket | null>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const thinking = status === 'thinking';
  const busy = thinking || status === 'running' || status === 'waiting';

  useEffect(() => {
    fetch('/api/agent/config')
      .then((r) => r.json())
      .then((s) => setModel(s.model || ''))
      .catch(() => {});
  }, [settingsOpen]);

  useEffect(() => {
    let closed = false;
    const tok = () => localStorage.getItem(PULSE_TOKEN) ?? '';
    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws';
      const q = tok() ? `?token=${encodeURIComponent(tok())}` : '';
      const sock = new WebSocket(`${proto}://${location.host}/api/agent/stream${q}`);
      ws.current = sock;
      sock.onopen = () => setWsState('open');
      sock.onclose = () => {
        setWsState('closed');
        if (!closed) setTimeout(connect, 3000);
      };
      sock.onmessage = (e) => {
        const m = JSON.parse(e.data);
        switch (m.type) {
          case 'status':
            setStatus(m.state);
            if (m.state === 'error' && m.text)
              setItems((it) => [...it, { kind: 'agent', text: `⚠️ ${m.text}`, time: now() }]);
            break;
          case 'assistant':
            setItems((it) => [...it, { kind: 'agent', text: m.text, time: now() }]);
            break;
          case 'tool':
            if (m.state === 'running') {
              setItems((it) => [
                ...it,
                { kind: 'tool', id: m.id, name: m.name, input: m.input ?? '', status: 'running' },
              ]);
            } else {
              setItems((it) =>
                it.map((x) =>
                  x.kind === 'tool' && x.id === m.id
                    ? { ...x, status: 'done', output: m.output, exit: m.exit ?? 0 }
                    : x
                )
              );
            }
            break;
          case 'confirm':
            setConfirm({ id: m.id, command: m.command, reason: m.reason });
            break;
        }
      };
    };
    fetch('/api/agent/config')
      .then((r) => r.json())
      .then((s) => {
        if (closed) return;
        if (s.auth && !tok()) {
          setGate(true);
          setWsState('closed');
          return;
        }
        connect();
      })
      .catch(() => connect());
    return () => {
      closed = true;
      ws.current?.close();
    };
  }, [attempt]);

  useEffect(() => {
    // Diqqat: strelka effekt tanasidan qiymat qaytarmasin — React qaytgan qiymatni
    // tozalash funksiyasi deb chaqiradi. Blok tanasi doim `undefined` qaytaradi.
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [items, thinking, confirm]);
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
    setItems((it) => [...it, { kind: 'user', text: t, time: now() }]);
    ws.current.send(JSON.stringify({ type: 'chat', text: t }));
    setInput('');
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
      setPw('');
      setGate(false);
      setAttempt((a) => a + 1);
    } catch {
      setAuthErr('Ulanib bo‘lmadi');
    }
  };

  if (gate) {
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

  const empty = items.length === 0;

  return (
    <div className="flex h-[calc(100vh-9rem)] min-h-[34rem] flex-col overflow-hidden rounded-2xl border border-line bg-surface">
      {/* Ingichka sarlavha */}
      <header className="flex shrink-0 items-center gap-2.5 border-b border-line px-4 py-2.5">
        <span className="flex size-6 items-center justify-center rounded-lg bg-accent/15 text-accent">
          <Bot className="size-3.5" />
        </span>
        <span className="text-sm font-medium">Agent</span>
        <span className={`size-1.5 rounded-full ${busy ? 'bg-warn' : 'bg-good'}`} />
        <span className="text-[11px] text-ink-muted">{busy ? 'ishlayapti' : 'tayyor'}</span>
        <button
          onClick={() => setSettingsOpen(true)}
          className="ml-auto flex items-center gap-1.5 rounded-lg border border-line px-2.5 py-1 text-xs text-ink-secondary transition-colors hover:bg-white/[0.04]"
        >
          <Cpu className="size-3 text-accent" />
          {model || 'model'}
          <SettingsIcon className="size-3 text-ink-muted" />
        </button>
      </header>

      {/* Suhbat */}
      <div className="pp-scroll flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-5 px-4 py-6">
          {empty && (
            <div className="flex flex-col items-center gap-4 py-10 text-center">
              <span className="flex size-12 items-center justify-center rounded-2xl bg-accent/15 text-accent">
                <Bot className="size-6" />
              </span>
              <div>
                <p className="text-sm font-medium">ParkPulse DevOps agenti</p>
                <p className="mt-1 text-xs text-ink-muted">
                  Tashxis qo‘yaman, config tahrirlayman, konteynerlarni tekshiraman. Xavfli amallar tasdiq so‘raydi.
                </p>
              </div>
              <div className="flex flex-wrap justify-center gap-2">
                {SUGGESTIONS.map((s) => (
                  <button
                    key={s}
                    onClick={() => send(s)}
                    className="rounded-full border border-line px-3 py-1.5 text-xs text-ink-secondary transition-colors hover:border-accent/40 hover:text-ink"
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
          )}

          {items.map((it, i) =>
            it.kind === 'user' ? (
              <div key={i} className="flex justify-end">
                <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-tr-md bg-accent/15 px-4 py-2.5 text-sm leading-relaxed text-ink">
                  {it.text}
                </div>
              </div>
            ) : it.kind === 'agent' ? (
              <div key={i} className="flex gap-3">
                <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-lg bg-accent/15 text-accent ring-1 ring-accent/20">
                  <Bot className="size-3.5" />
                </span>
                <p className="whitespace-pre-wrap pt-0.5 text-sm leading-relaxed text-ink-secondary">{it.text}</p>
              </div>
            ) : (
              <div key={i} className="pl-9">
                <ToolStep item={it} />
              </div>
            )
          )}

          {thinking && (
            <div className="flex items-center gap-3">
              <span className="flex size-6 items-center justify-center rounded-lg bg-accent/15 text-accent">
                <Bot className="size-3.5" />
              </span>
              <span className="flex items-center gap-1.5 text-xs text-ink-muted">
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '0ms' }} />
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '150ms' }} />
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '300ms' }} />
                o‘ylayapti…
              </span>
            </div>
          )}

          {confirm && (
            <div className="ml-9 rounded-xl border border-warn/40 bg-warn/[0.08] p-3">
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
          <div ref={endRef} />
        </div>
      </div>

      {/* Composer */}
      <div className="border-t border-line px-4 py-3">
        <div className="mx-auto max-w-3xl">
          <div className="flex items-end gap-2 rounded-2xl border border-line bg-page px-3 py-2 focus-within:border-accent/60">
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
              placeholder={wsState === 'open' ? 'Agentga xabar yozing…' : 'Ulanmoqda…'}
              className="max-h-40 flex-1 resize-none bg-transparent py-1.5 text-sm text-ink placeholder:text-ink-muted focus:outline-none disabled:opacity-50"
            />
            <button
              onClick={() => send(input)}
              disabled={wsState !== 'open' || !input.trim()}
              className="mb-0.5 flex size-8 shrink-0 items-center justify-center rounded-xl bg-accent text-white transition-opacity hover:opacity-90 disabled:opacity-40"
              aria-label="Yuborish"
            >
              <ArrowUp className="size-4" />
            </button>
          </div>
          {wsState === 'closed' && !gate && (
            <p className="mt-2 text-center text-xs text-ink-muted">Ulanish uzildi — qayta urinmoqda…</p>
          )}
        </div>
      </div>

      {settingsOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-black/60 p-4 sm:p-8">
          <div className="w-full max-w-2xl">
            <div className="mb-3 flex justify-end">
              <button
                onClick={() => setSettingsOpen(false)}
                className="rounded-lg border border-line bg-surface p-1.5 text-ink-muted hover:text-ink"
                aria-label="Yopish"
              >
                <X className="size-4" />
              </button>
            </div>
            <AgentSettings />
          </div>
        </div>
      )}
    </div>
  );
}

function iconFor(name: string) {
  if (name === 'bash') return Terminal;
  if (name.startsWith('read') || name.startsWith('write')) return FileText;
  if (name.startsWith('docker')) return Boxes;
  return Wrench;
}

function ToolStep({ item }: { item: ToolItem }) {
  const [open, setOpen] = useState(false);
  const Icon = iconFor(item.name);
  const hasOut = !!item.output;
  return (
    <div className="rounded-xl border border-line bg-page/60">
      <button
        onClick={() => hasOut && setOpen((o) => !o)}
        className={`flex w-full items-center gap-2 px-3 py-2 text-left ${hasOut ? 'cursor-pointer' : 'cursor-default'}`}
      >
        <Icon className="size-3.5 shrink-0 text-ink-muted" />
        <span className="truncate font-mono text-xs text-ink-secondary">
          {item.name}
          {item.input ? ` ${item.input}` : ''}
        </span>
        <span className="ml-auto shrink-0">
          {item.status === 'running' ? (
            <Loader2 className="size-3.5 animate-spin text-ink-muted" />
          ) : item.exit === 0 ? (
            <Check className="size-3.5 text-good" />
          ) : (
            <X className="size-3.5 text-critical" />
          )}
        </span>
        {hasOut && <ChevronDown className={`size-3.5 shrink-0 text-ink-muted transition-transform ${open ? 'rotate-180' : ''}`} />}
      </button>
      {open && hasOut && (
        <pre className="pp-scroll max-h-64 overflow-auto border-t border-line px-3 py-2 font-mono text-[11px] leading-relaxed text-ink-muted">
          {item.output}
        </pre>
      )}
    </div>
  );
}
