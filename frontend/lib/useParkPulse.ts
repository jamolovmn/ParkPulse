'use client';

import { useEffect, useRef, useState } from 'react';

export type Stats = { total_passes: number; avg_latency_ms: number; ghost_count: number };
export type Breakdown = { gateway_ms: number; db_ms: number; pos_ms: number };
export type Pass = {
  plate: string;
  gate: string;
  anpr_at: string;
  relay_at: string;
  latency_ms: number;
  breakdown?: Breakdown;
};
export type Ghost = {
  plate?: string;
  gate: string;
  relay_at: string;
  raw: string;
  context?: string[]; // aniqlangan paytdagi atrofdagi log qatorlari
};

export type Device = {
  name: string;
  ip: string;
  alive: boolean;
  rtt_ms: number;
  last_seen?: string;
};

const LIMIT = 50;

export function useParkPulse() {
  const [connected, setConnected] = useState(false);
  const [stats, setStats] = useState<Stats>({ total_passes: 0, avg_latency_ms: 0, ghost_count: 0 });
  const [passes, setPasses] = useState<Pass[]>([]); // eng yangisi birinchi
  const [ghosts, setGhosts] = useState<Ghost[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
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
            setStats(msg.data.stats);
            setPasses([...(msg.data.passes ?? [])].reverse());
            setGhosts([...(msg.data.ghosts ?? [])].reverse());
            setDevices(msg.data.devices ?? []);
            break;
          case 'devices':
            setDevices(msg.data ?? []);
            break;
          case 'stats':
            setStats(msg.data);
            break;
          case 'pass':
            setPasses((p) => [msg.data, ...p].slice(0, LIMIT));
            break;
          case 'ghost':
            setGhosts((g) => [msg.data, ...g].slice(0, LIMIT));
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

  return { connected, stats, passes, ghosts, devices };
}
