'use client';

import { useState } from 'react';
import { useParkPulse, Speed, Stats } from '@/lib/useParkPulse';
import TrafficChart from '@/components/TrafficChart';
import LiveFeed from '@/components/LiveFeed';
import OpenList from '@/components/OpenList';
import Devices from '@/components/Devices';
import SnmpPanel from '@/components/SnmpPanel';
import AlertSettings from '@/components/AlertSettings';
import { HealthPanel, ContainerTable } from '@/components/Health';

const fmtMs = (ms: number) => `${ms < 10 ? ms.toFixed(1) : String(Math.round(ms))} ms`;

type Section = 'dashboard' | 'devices' | 'network' | 'system';
type Panel = 'passes' | 'opens';

type NavItem = { id: Section; label: string; icon: string };

const NAV: NavItem[] = [
  { id: 'dashboard', label: 'Boshqaruv', icon: '◈' },
  { id: 'devices', label: 'Qurilmalar', icon: '⌸' },
  { id: 'system', label: 'Tizim', icon: '⚙' },
];

export default function Dashboard() {
  const { connected, stats, passes, opens, ghosts, traffic, devices, snmp, speed, health } =
    useParkPulse();
  const [section, setSection] = useState<Section>('dashboard');
  const [panel, setPanel] = useState<Panel>('passes');

  const offline = devices.filter((d) => !d.alive).length;

  // "Tarmoq" bo'limi faqat SNMP sozlangan bo'lsa ko'rinadi (bo'sh menyu bo'lmasin).
  const nav = snmp.length > 0 ? [...NAV.slice(0, 2), { id: 'network' as const, label: 'Tarmoq', icon: '⇅' }, NAV[2]] : NAV;

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6 lg:flex-row lg:gap-8 lg:py-8">
      <Sidebar
        nav={nav}
        section={section}
        onSelect={setSection}
        connected={connected}
        offline={offline}
      />

      <main className="min-w-0 flex-1 space-y-6">
        <Header connected={connected} speed={speed} />

        {section === 'dashboard' && (
          <>
            <KpiRow stats={stats} panel={panel} onPanel={setPanel} />
            <TrafficChart points={traffic} />
            {panel === 'passes' ? (
              <LiveFeed passes={passes} />
            ) : (
              <OpenList opens={opens} ghosts={ghosts} />
            )}
          </>
        )}

        {section === 'devices' && <Devices devices={devices} />}

        {section === 'network' && <SnmpPanel hosts={snmp} />}

        {section === 'system' && (
          <div className="space-y-6">
            <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
              <HealthPanel health={health} />
              <ContainerTable health={health} />
            </div>
            <AlertSettings />
          </div>
        )}
      </main>
    </div>
  );
}

function Sidebar({
  nav,
  section,
  onSelect,
  connected,
  offline,
}: {
  nav: NavItem[];
  section: Section;
  onSelect: (s: Section) => void;
  connected: boolean;
  offline: number;
}) {
  return (
    <aside className="lg:w-52 lg:shrink-0">
      <div className="mb-6 hidden items-center gap-2.5 lg:flex">
        <span className={`h-2.5 w-2.5 rounded-full ${connected ? 'bg-good' : 'bg-critical'}`} />
        <div>
          <p className="text-sm font-semibold tracking-tight">ParkPulse</p>
          <p className="text-[11px] text-ink-muted">Smart Parking</p>
        </div>
      </div>

      <nav className="flex gap-1 overflow-x-auto lg:flex-col">
        {nav.map((n) => {
          const active = section === n.id;
          return (
            <button
              key={n.id}
              onClick={() => onSelect(n.id)}
              className={`flex shrink-0 items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors ${
                active
                  ? 'bg-white/[0.07] font-medium text-ink'
                  : 'text-ink-muted hover:bg-white/[0.03] hover:text-ink-secondary'
              }`}
            >
              <span aria-hidden className="text-base leading-none">
                {n.icon}
              </span>
              {n.label}
              {n.id === 'devices' && offline > 0 && (
                <span className="ml-auto rounded-full bg-critical/15 px-1.5 text-[10px] font-medium text-critical">
                  {offline}
                </span>
              )}
            </button>
          );
        })}
      </nav>
    </aside>
  );
}

function Header({ connected, speed }: { connected: boolean; speed: Speed | null }) {
  return (
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div className="lg:hidden">
        <h1 className="text-lg font-semibold tracking-tight">ParkPulse</h1>
      </div>
      <div className="ml-auto flex items-center gap-4">
        {speed && (
          <span
            className="text-xs text-ink-secondary [font-variant-numeric:tabular-nums]"
            title="Server internet tezligi"
          >
            ↓ {speed.download_mbps.toFixed(0)} · ↑ {speed.upload_mbps.toFixed(0)} Mbps ·{' '}
            {Math.round(speed.ping_ms)} ms
          </span>
        )}
        <span
          className={`h-2.5 w-2.5 rounded-full lg:hidden ${connected ? 'bg-good' : 'bg-critical'}`}
          title={connected ? 'Ulangan' : 'Uzilgan'}
        />
      </div>
    </header>
  );
}

function KpiRow({
  stats,
  panel,
  onPanel,
}: {
  stats: Stats;
  panel: Panel;
  onPanel: (p: Panel) => void;
}) {
  const opens = stats.opens ?? {};
  const clean = (opens.paid ?? 0) + (opens.remote ?? 0) + (opens.entry ?? 0);

  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Kpi
        label="O'rtacha latency"
        value={stats.total_passes ? fmtMs(stats.avg_latency_ms) : '—'}
        note={`${stats.total_passes} ta o'tish`}
        active={panel === 'passes'}
        onClick={() => onPanel('passes')}
      />
      <Kpi
        label="Arvoh ochilishlar"
        value={String(stats.ghost_count)}
        note="Qarzdor + mashinasiz"
        alert={stats.ghost_count > 0}
        active={panel === 'opens'}
        onClick={() => onPanel('opens')}
      />
      <Kpi label="Muammosiz ochilish" value={String(clean)} note="To'landi + pult" />
      <Kpi
        label="Qoidabuzarlik"
        value={String(opens.violation ?? 0)}
        note={`Mashinasiz: ${opens.ghost ?? 0}`}
        alert={(opens.violation ?? 0) > 0}
      />
    </div>
  );
}

function Kpi({
  label,
  value,
  note,
  alert,
  active,
  onClick,
}: {
  label: string;
  value: string;
  note?: string;
  alert?: boolean;
  active?: boolean;
  onClick?: () => void;
}) {
  const clickable = !!onClick;
  return (
    <div
      onClick={onClick}
      role={clickable ? 'button' : undefined}
      className={`rounded-xl border p-4 transition-colors ${
        clickable ? 'cursor-pointer' : ''
      } ${
        active
          ? 'border-ink/25 bg-surface'
          : `border-line bg-surface/50 ${clickable ? 'hover:border-ink/20 hover:bg-surface' : ''}`
      }`}
    >
      <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">{label}</p>
      <p
        className={`mt-1.5 text-2xl font-semibold [font-variant-numeric:tabular-nums] ${
          alert ? 'text-critical' : 'text-ink'
        }`}
      >
        {value}
      </p>
      {note && <p className="mt-0.5 text-[11px] text-ink-muted">{note}</p>}
    </div>
  );
}
