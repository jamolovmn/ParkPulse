'use client';

import { useEffect, useRef, useState } from 'react';

/** Shlagbaum ochilishining turlari (backend: analyzer.OpenKind). */
export type OpenKind = 'paid' | 'remote' | 'entry' | 'violation' | 'ghost';

/** Faqat shu ikkisi haqiqiy "arvoh ochilish" — loglanadi va qizil ko'rsatiladi. */
export const isSuspicious = (k: OpenKind) => k === 'violation' || k === 'ghost';

export type Stats = {
  total_passes: number;
  avg_latency_ms: number;
  ghost_count: number;
  opens: Partial<Record<OpenKind, number>>;
};

export type Breakdown = { gateway_ms: number; db_ms: number; pos_ms: number };

export type Pass = {
  plate: string;
  gate: string;
  anpr_at: string;
  relay_at: string;
  latency_ms: number;
  breakdown?: Breakdown;
  /** To'lov ochilishdan keyin olingan (pult) — latency o'rtachaga kirmaydi. */
  auto_pay?: boolean;
};

export type Open = {
  kind: OpenKind;
  plate?: string;
  gate: string;
  open_at: string;
  reason: string;
  raw: string;
  context?: string[]; // faqat shubhali turlar uchun
};

export type TrafficPoint = { hour: string; enter: number; exit: number };

export type Device = {
  name: string;
  ip: string;
  alive: boolean;
  rtt_ms: number;
  last_seen?: string;
  type?: string; // avto aniqlangan: Kamera / Web qurilma / Noma'lum
  vendor?: string; // Hikvision, Dahua...
  ports?: number[];
  watched?: boolean; // uzilganda alert bersinmi (★)
  // Sifat ko'rsatkichlari (so'nggi ~5 daqiqa)
  min_ms?: number;
  avg_ms?: number;
  max_ms?: number;
  jitter_ms?: number;
  loss_pct?: number;
  uptime_pct?: number;
  samples?: number[]; // sparkline: RTT, -1 = javobsiz
};

export type Speed = { ping_ms: number; download_mbps: number; upload_mbps: number };

/** SNMP interfeysi (switch/router porti). */
export type SnmpIface = {
  index: number;
  name: string;
  up: boolean;
  in_mbps: number;
  out_mbps: number;
  speed_mbps?: number;
};

/** SNMP qurilmasi. */
export type SnmpHost = {
  name: string;
  ip: string;
  up: boolean;
  descr?: string;
  uptime?: string;
  ifaces: SnmpIface[];
  err?: string;
};

export type ContainerStat = {
  name: string;
  cpu_percent: number;
  ram_percent: number;
  ram_mb: number;
};

export type SystemHealth = {
  uptime_sec: number;
  cores: number[];
  containers: ContainerStat[];
  total_ram_mb: number;
  used_ram_mb: number;
};

const LIMIT = 50;
// Shubhali ochilishlar kam, lekin muhim — ularni uzoqroq saqlaymiz, aks holda
// KPI "1 ta arvoh" desa-yu, ro'yxat bo'sh chiqib qoladi (band darvozada 50 talik
// umumiy oyna arvohni tezda siqib chiqaradi).
const GHOST_LIMIT = 200;

const emptyStats: Stats = { total_passes: 0, avg_latency_ms: 0, ghost_count: 0, opens: {} };

export function useParkPulse() {
  const [connected, setConnected] = useState(false);
  const [stats, setStats] = useState<Stats>(emptyStats);
  const [passes, setPasses] = useState<Pass[]>([]);
  const [opens, setOpens] = useState<Open[]>([]);
  const [ghosts, setGhosts] = useState<Open[]>([]);
  const [traffic, setTraffic] = useState<TrafficPoint[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [snmp, setSnmp] = useState<SnmpHost[]>([]);
  const [speed, setSpeed] = useState<Speed | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    let ws: WebSocket;
    let closed = false;

    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws';
      ws = new WebSocket(`${proto}://${location.host}/ws`);

      ws.onopen = () => setConnected(true);
      ws.onmessage = (e) => {
        const msg = JSON.parse(e.data);
        switch (msg.type) {
          case 'snapshot':
            setStats({ ...emptyStats, ...msg.data.stats });
            setPasses([...(msg.data.passes ?? [])].reverse());
            setOpens([...(msg.data.opens ?? [])].reverse());
            setGhosts([...(msg.data.ghosts ?? [])].reverse());
            setTraffic(msg.data.traffic ?? []);
            setDevices(msg.data.devices ?? []);
            setSnmp(msg.data.snmp ?? []);
            setSpeed(msg.data.speed ?? null);
            setHealth(msg.data.health ?? null);
            break;
          case 'stats':
            setStats({ ...emptyStats, ...msg.data });
            break;
          case 'pass':
            setPasses((p) => [msg.data, ...p].slice(0, LIMIT));
            break;
          case 'open':
            setOpens((o) => [msg.data, ...o].slice(0, LIMIT));
            if (isSuspicious(msg.data.kind))
              setGhosts((g) => [msg.data, ...g].slice(0, GHOST_LIMIT));
            break;
          case 'traffic':
            setTraffic(msg.data ?? []);
            break;
          case 'devices':
            setDevices(msg.data ?? []);
            break;
          case 'snmp':
            setSnmp(msg.data ?? []);
            break;
          case 'speedtest':
            setSpeed(msg.data ?? null);
            break;
          case 'health':
            setHealth(msg.data);
            break;
        }
      };
      ws.onclose = () => {
        setConnected(false);
        if (!closed) timer.current = setTimeout(connect, 3000); // qayta ulanish
      };
    };

    connect();
    return () => {
      closed = true;
      clearTimeout(timer.current);
      ws?.close();
    };
  }, []);

  return { connected, stats, passes, opens, ghosts, traffic, devices, snmp, speed, health };
}
