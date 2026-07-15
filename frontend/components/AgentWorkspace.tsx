'use client';

import { useEffect, useRef, useState } from 'react';
import {
  Bot,
  Settings as SettingsIcon,
  ArrowUp,
  Terminal as TerminalIcon,
  Circle,
  ShieldAlert,
  X,
  Cpu,
} from 'lucide-react';
import AgentSettings, { PULSE_TOKEN } from '@/components/AgentSettings';

// Backend WS protokoli bilan bir xil (status/assistant/log/tool/confirm ...).

type ChatItem = { role: 'user' | 'agent'; text: string; time: string };
type Level = 'command' | 'info' | 'output' | 'error' | 'ok';
type Line = { level: Level; text: string };
type Confirm = { id: string; command: string; reason: string };

const now = () => new Date().toLocaleTimeString('uz-UZ', { hour: '2-digit', minute: '2-digit' });

export default function AgentWorkspace() {
  const [chat, setChat] = useState<ChatItem[]>([]);
  const [lines, setLines] = useState<Line[]>([]);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [status, setStatus] = useState('idle');
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'closed'>('connecting');
  const [input, setInput] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [model, setModel] = useState('');

  // Auth
  const [gate, setGate] = useState(false);
  const [pw, setPw] = useState('');
  const [authErr, setAuthErr] = useState('');
  const [attempt, setAttempt] = useState(0);

  const ws = useRef<WebSocket | null>(null);
  const chatEnd = useRef<HTMLDivElement>(null);
  const termEnd = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const running = status === 'thinking' || status === 'running' || status === 'waiting';
  const thinking = status === 'thinking';

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
            if (m.state === 'error' && m.text) push('error', m.text);
            break;
          case 'assistant':
            setChat((c) => [...c, { role: 'agent', text: m.text, time: now() }]);
            break;
          case 'log':
            push('info', m.text);
            break;
          case 'tool':
            if (m.state === 'running') push('command', `${m.name} ${m.input ?? ''}`.trim());
            else if (m.output) push(m.exit && m.exit !== 0 ? 'error' : 'output', m.output.replace(/\n$/, ''));
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

  useEffect(() => chatEnd.current?.scrollIntoView({ behavior: 'smooth' }), [chat, thinking]);
  useEffect(() => termEnd.current?.scrollIntoView({ behavior: 'smooth' }), [lines, confirm]);
  useEffect(() => {
    const ta = taRef.current;
    if (ta) {
      ta.style.height = '0px';
      ta.style.height = Math.min(ta.scrollHeight, 140) + 'px';
    }
  }, [input]);

  const push = (level: Level, text: string) => setLines((l) => [...l, { level, text }]);

  const send = () => {
    const text = input.trim();
    if (!text || ws.current?.readyState !== WebSocket.OPEN) return;
    setChat((c) => [...c, { role: 'user', text, time: now() }]);
    ws.current.send(JSON.stringify({ type: 'chat', text }));
    setInput('');
  };

  const decide = (approve: boolean) => {
    if (!confirm) return;
    ws.current?.send(JSON.stringify({ type: 'decision', id: confirm.id, approve }));
    push(approve ? 'ok' : 'error', approve ? '✓ tasdiqlandi' : '✗ rad etildi');
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
      <section className="mx-auto max-w-md rounded-xl border border-line bg-surface p-6">
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

  return (
    <div className="flex h-[calc(100vh-9rem)] min-h-[34rem] flex-col overflow-hidden rounded-xl border border-line bg-surface">
      {/* Titlebar */}
      <header className="flex h-11 shrink-0 items-center gap-3 border-b border-line bg-page/40 px-3">
        <div className="flex items-center gap-1.5">
          <span className="size-3 rounded-full bg-[#ff5f57]" />
          <span className="size-3 rounded-full bg-[#febc2e]" />
          <span className="size-3 rounded-full bg-[#28c840]" />
        </div>
        <div className="ml-1 flex items-center gap-2">
          <span className="flex size-5 items-center justify-center rounded bg-accent/15 text-accent">
            <Bot className="size-3.5" />
          </span>
          <span className="text-sm font-medium">ParkPulse Agent</span>
        </div>
        <span
          className={`ml-auto flex items-center gap-1.5 rounded-md px-2 py-1 text-[11px] ${
            running ? 'bg-warn/15 text-warn' : 'bg-good/15 text-good'
          }`}
        >
          <Circle className="size-2 fill-current" />
          {running ? 'Agent ishlayapti' : 'Agent tayyor'}
        </span>
        <button
          onClick={() => setSettingsOpen(true)}
          title="Sozlamalar"
          className="rounded-md p-1.5 text-ink-muted transition-colors hover:bg-white/[0.06] hover:text-ink"
        >
          <SettingsIcon className="size-4" />
        </button>
      </header>

      {/* Body: chat | terminal */}
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Chat */}
        <section className="flex min-h-0 flex-1 flex-col border-b border-line lg:w-[440px] lg:flex-none lg:border-b-0 lg:border-r">
          <div className="pp-scroll flex-1 space-y-5 overflow-y-auto px-4 py-4">
            {chat.length === 0 && (
              <p className="text-sm text-ink-muted">
                Savol bering — “Nega oxirgi konteyner qulab tushdi?”, “p24.json’da enter 1 IP’sini o‘zgartir”.
                Xavfli buyruqlar tasdiq so‘raydi.
              </p>
            )}
            {chat.map((c, i) =>
              c.role === 'user' ? (
                <div key={i} className="flex justify-end">
                  <div className="max-w-[88%] rounded-xl rounded-tr-sm bg-accent/15 px-3.5 py-2 text-sm leading-relaxed text-ink">
                    {c.text}
                  </div>
                </div>
              ) : (
                <div key={i} className="flex flex-col gap-1.5">
                  <div className="flex items-center gap-2">
                    <span className="flex size-5 items-center justify-center rounded-md bg-accent/15 text-accent ring-1 ring-accent/25">
                      <Bot className="size-3" />
                    </span>
                    <span className="text-xs font-medium">Agent</span>
                    <span className="font-mono text-[10px] text-ink-muted">{c.time}</span>
                  </div>
                  <p className="whitespace-pre-wrap pl-7 text-sm leading-relaxed text-ink-secondary">{c.text}</p>
                </div>
              )
            )}
            {thinking && (
              <div className="flex items-center gap-1.5 pl-7 text-xs text-ink-muted">
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '0ms' }} />
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '150ms' }} />
                <span className="pp-typing-dot size-1.5 rounded-full bg-accent" style={{ animationDelay: '300ms' }} />
                <span className="ml-1">o‘ylayapti…</span>
              </div>
            )}
            <div ref={chatEnd} />
          </div>

          {/* Composer */}
          <div className="border-t border-line p-3">
            <div className="rounded-xl border border-line bg-page focus-within:border-accent/60">
              <textarea
                ref={taRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    send();
                  }
                }}
                rows={1}
                placeholder={wsState === 'open' ? 'Agentga xabar yozing…' : 'Ulanmoqda…'}
                disabled={wsState !== 'open'}
                className="max-h-36 w-full resize-none bg-transparent px-3 py-2.5 text-sm text-ink placeholder:text-ink-muted focus:outline-none disabled:opacity-50"
              />
              <div className="flex items-center gap-2 px-2.5 pb-2">
                <button
                  onClick={() => setSettingsOpen(true)}
                  className="flex items-center gap-1 rounded-md px-1.5 py-1 text-[11px] font-medium text-ink-muted transition-colors hover:bg-white/[0.06] hover:text-ink"
                >
                  <Cpu className="size-3 text-accent" />
                  {model || 'model tanlang'}
                </button>
                <button
                  onClick={send}
                  disabled={wsState !== 'open' || !input.trim()}
                  className="ml-auto flex size-7 items-center justify-center rounded-lg bg-accent text-white transition-opacity hover:opacity-90 disabled:opacity-40"
                  aria-label="Yuborish"
                >
                  <ArrowUp className="size-4" />
                </button>
              </div>
            </div>
            {wsState === 'closed' && !gate && (
              <p className="mt-2 text-xs text-ink-muted">Ulanish uzildi — qayta urinmoqda…</p>
            )}
          </div>
        </section>

        {/* Terminal */}
        <section className="flex min-h-0 flex-1 flex-col bg-term-bg">
          <div className="flex items-center gap-2 border-b border-line bg-page/40 px-3 py-2 text-sm">
            <TerminalIcon className="size-4 text-accent" />
            <span className="font-medium">Jonli ijro</span>
            <span className="ml-auto flex items-center gap-1.5 text-xs text-ink-muted">
              <Circle className={`size-2 fill-current ${running ? 'text-warn' : 'text-good'}`} />
              {running ? 'running' : 'idle'}
            </span>
          </div>
          <div className="pp-scroll flex-1 overflow-y-auto px-4 py-3 font-mono text-[13px] leading-relaxed">
            {lines.length === 0 && <p className="text-ink-muted">Agent buyruqlari shu yerda oqadi…</p>}
            {lines.map((l, i) => (
              <div key={i} className="whitespace-pre-wrap break-words">
                <HL line={l} />
              </div>
            ))}
            {running && (
              <div className="flex items-center gap-2 pt-1">
                <span className="text-term-green">$</span>
                <span className="pp-cursor inline-block h-4 w-2 bg-ink align-middle" />
              </div>
            )}
            <div ref={termEnd} />
          </div>

          {confirm && (
            <div className="border-t border-warn/40 bg-warn/[0.08] px-4 py-3">
              <p className="flex items-center gap-1.5 text-xs font-medium text-warn">
                <ShieldAlert className="size-3.5" /> Tasdiq kerak (xavfli buyruq)
              </p>
              <pre className="mt-1.5 whitespace-pre-wrap font-mono text-xs text-ink">{confirm.command}</pre>
              <p className="mt-1 text-xs text-ink-muted">{confirm.reason}</p>
              <div className="mt-2.5 flex gap-2">
                <button onClick={() => decide(true)} className="rounded-lg bg-critical px-3 py-1.5 text-xs font-medium text-white hover:opacity-90">
                  Ha, bajar (Y)
                </button>
                <button onClick={() => decide(false)} className="rounded-lg border border-line px-3 py-1.5 text-xs text-ink-secondary hover:bg-white/[0.04]">
                  Yo‘q (N)
                </button>
              </div>
            </div>
          )}
        </section>
      </div>

      {/* Status bar */}
      <footer className="flex h-7 shrink-0 items-center gap-4 border-t border-line bg-accent/90 px-3 font-mono text-[11px] text-white">
        <span className="flex items-center gap-1.5">
          <Bot className="size-3.5" /> {model || 'model tanlanmagan'}
        </span>
        <span className="ml-auto">{lines.length} qator</span>
        <span>{running ? 'streaming…' : status}</span>
      </footer>

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

// Bitta log qatorini rang bilan (buyruq / kalit so'z / raqam).
function HL({ line }: { line: Line }) {
  if (line.level === 'command') {
    return (
      <span className="flex gap-2">
        <span className="select-none text-term-green">$</span>
        <span className="text-ink">{line.text}</span>
      </span>
    );
  }
  const tone =
    line.level === 'error'
      ? 'text-term-red'
      : line.level === 'ok'
        ? 'text-term-green'
        : line.level === 'output'
          ? 'text-term-cyan'
          : 'text-ink-muted';
  const parts = line.text.split(/(\bERROR\b|\bWARN\b|\bINFO\b|\bFATAL\b|✓|✗|⚠|\b\d+(?:ms|s|Mi|Gi|%)\b)/g);
  return (
    <span className={tone}>
      {parts.map((t, i) => {
        if (t === 'ERROR' || t === 'FATAL' || t === '✗') return <span key={i} className="font-semibold text-term-red">{t}</span>;
        if (t === 'WARN' || t === '⚠') return <span key={i} className="font-semibold text-term-yellow">{t}</span>;
        if (t === 'INFO') return <span key={i} className="font-semibold text-term-blue">{t}</span>;
        if (t === '✓') return <span key={i} className="font-semibold text-term-green">{t}</span>;
        if (/^\d+(?:ms|s|Mi|Gi|%)$/.test(t)) return <span key={i} className="text-term-magenta">{t}</span>;
        return <span key={i}>{t}</span>;
      })}
    </span>
  );
}
