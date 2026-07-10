'use client';

import { useMemo, useRef, useState } from 'react';
import { TrafficPoint } from '@/lib/useParkPulse';

// Kategorik palitra 1- va 2-slot (dark), validator bilan tekshirilgan.
const ENTER = '#3987e5';
const EXIT = '#199e70';

const W = 720;
const H = 220;
const PAD = { l: 36, r: 56, t: 16, b: 26 };
const IW = W - PAD.l - PAD.r;
const IH = H - PAD.t - PAD.b;

const hourLabel = (iso: string) =>
  new Date(iso).toLocaleTimeString('uz-UZ', { hour: '2-digit', hour12: false });

/** niceMax — o'qni 1/2/5·10ⁿ qadamlariga yaxlitlaydi. */
function niceMax(v: number) {
  if (v <= 4) return 4;
  const pow = 10 ** Math.floor(Math.log10(v));
  const n = v / pow;
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10;
  return step * pow;
}

export default function TrafficChart({ points }: { points: TrafficPoint[] }) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);

  const { max, x, y, enterPath, exitPath, total } = useMemo(() => {
    const n = Math.max(points.length, 2);
    const peak = points.reduce((m, p) => Math.max(m, p.enter, p.exit), 0);
    const max = niceMax(peak);
    const x = (i: number) => PAD.l + (i * IW) / (n - 1);
    const y = (v: number) => PAD.t + IH - (v / max) * IH;
    const line = (pick: (p: TrafficPoint) => number) =>
      points.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(pick(p)).toFixed(1)}`).join(' ');
    return {
      max,
      x,
      y,
      enterPath: line((p) => p.enter),
      exitPath: line((p) => p.exit),
      total: points.reduce((s, p) => s + p.enter + p.exit, 0),
    };
  }, [points]);

  if (points.length === 0) {
    return (
      <Card>
        <p className="px-5 py-16 text-center text-sm text-ink-muted">Ma'lumot yig'ilmoqda…</p>
      </Card>
    );
  }

  const onMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const box = svgRef.current!.getBoundingClientRect();
    const px = ((e.clientX - box.left) / box.width) * W;
    const i = Math.round(((px - PAD.l) / IW) * (points.length - 1));
    setHover(i >= 0 && i < points.length ? i : null);
  };

  const hp = hover !== null ? points[hover] : null;
  // Tooltip chetdan chiqmasin: o'ng yarmida bo'lsa chapga yopishadi.
  const hx = hover !== null ? x(hover) : 0;
  const flip = hx > W * 0.62;

  const lastEnter = points[points.length - 1].enter;
  const lastExit = points[points.length - 1].exit;
  // Oxirgi nuqtada yorliqlar ustma-ust tushmasin.
  const collide = Math.abs(y(lastEnter) - y(lastExit)) < 14;

  return (
    <Card>
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3.5">
        <div>
          <h2 className="text-sm font-medium">Oxirgi 24 soat</h2>
          <p className="mt-0.5 text-xs text-ink-muted">Soatlik kirish va chiqish</p>
        </div>
        <div className="flex items-center gap-4 text-xs">
          <Legend color={ENTER} label="Kirish" />
          <Legend color={EXIT} label="Chiqish" />
          <span className="text-ink-muted [font-variant-numeric:tabular-nums]">
            Jami {total}
          </span>
        </div>
      </header>

      <div className="relative px-2 pb-2 pt-3">
        <svg
          ref={svgRef}
          viewBox={`0 0 ${W} ${H}`}
          className="h-auto w-full"
          role="img"
          aria-label={`Oxirgi 24 soatda ${total} ta mashina harakati`}
          onMouseMove={onMove}
          onMouseLeave={() => setHover(null)}
        >
          {/* Recessiv setka va y-o'qi */}
          {[0, 0.5, 1].map((f) => {
            const v = max * f;
            return (
              <g key={f}>
                <line
                  x1={PAD.l}
                  x2={PAD.l + IW}
                  y1={y(v)}
                  y2={y(v)}
                  stroke="#2c2c2a"
                  strokeWidth={1}
                />
                <text
                  x={PAD.l - 8}
                  y={y(v) + 4}
                  textAnchor="end"
                  className="fill-[#898781] text-[11px] [font-variant-numeric:tabular-nums]"
                >
                  {v}
                </text>
              </g>
            );
          })}

          {/* Har 4 soatda vaqt yorlig'i */}
          {points.map((p, i) =>
            i % 4 === 0 || i === points.length - 1 ? (
              <text
                key={p.hour}
                x={x(i)}
                y={H - 8}
                textAnchor={i === points.length - 1 ? 'end' : 'middle'}
                className="fill-[#898781] text-[11px] [font-variant-numeric:tabular-nums]"
              >
                {hourLabel(p.hour)}
              </text>
            ) : null
          )}

          {hover !== null && (
            <line x1={hx} x2={hx} y1={PAD.t} y2={PAD.t + IH} stroke="#898781" strokeWidth={1} />
          )}

          <path d={exitPath} fill="none" stroke={EXIT} strokeWidth={2} strokeLinejoin="round" />
          <path d={enterPath} fill="none" stroke={ENTER} strokeWidth={2} strokeLinejoin="round" />

          {/* To'g'ridan-to'g'ri yorliqlar — identifikatsiya rangga bog'liq qolmasin */}
          <text x={x(points.length - 1) + 8} y={y(lastEnter) + (collide ? -4 : 4)} fill={ENTER} className="text-[11px] font-medium">
            Kirish
          </text>
          <text x={x(points.length - 1) + 8} y={y(lastExit) + (collide ? 14 : 4)} fill={EXIT} className="text-[11px] font-medium">
            Chiqish
          </text>

          {hover !== null && hp && (
            <>
              <circle cx={hx} cy={y(hp.exit)} r={5} fill={EXIT} stroke="#1a1a19" strokeWidth={2} />
              <circle cx={hx} cy={y(hp.enter)} r={5} fill={ENTER} stroke="#1a1a19" strokeWidth={2} />
            </>
          )}
        </svg>

        {hover !== null && hp && (
          <div
            className="pointer-events-none absolute top-4 rounded-md border border-line bg-page px-3 py-2 text-xs shadow-lg"
            style={{
              left: `${((hx / W) * 100).toFixed(2)}%`,
              transform: `translateX(${flip ? 'calc(-100% - 12px)' : '12px'})`,
            }}
          >
            <p className="mb-1 font-medium [font-variant-numeric:tabular-nums]">
              {hourLabel(hp.hour)}
            </p>
            <p className="flex items-center gap-2 text-ink-secondary">
              <Dot color={ENTER} /> Kirish
              <span className="ml-auto [font-variant-numeric:tabular-nums]">{hp.enter}</span>
            </p>
            <p className="flex items-center gap-2 text-ink-secondary">
              <Dot color={EXIT} /> Chiqish
              <span className="ml-auto [font-variant-numeric:tabular-nums]">{hp.exit}</span>
            </p>
          </div>
        )}
      </div>
    </Card>
  );
}

function Card({ children }: { children: React.ReactNode }) {
  return <section className="rounded-xl border border-line bg-surface">{children}</section>;
}

function Dot({ color }: { color: string }) {
  return <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: color }} />;
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1.5 text-ink-secondary">
      <Dot color={color} />
      {label}
    </span>
  );
}
