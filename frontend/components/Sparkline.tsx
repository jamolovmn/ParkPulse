'use client';

// Sparkline — qurilma RTT tarixining mitti grafigi. Manfiy qiymat (-1) javobsiz
// pingni bildiradi va qizil nuqta bilan ko'rsatiladi.
export default function Sparkline({
  data,
  width = 96,
  height = 24,
}: {
  data: number[];
  width?: number;
  height?: number;
}) {
  if (!data || data.length < 2) return null;

  const alive = data.filter((v) => v >= 0);
  const max = Math.max(1, ...alive);
  const n = data.length;
  const x = (i: number) => (i / (n - 1)) * (width - 2) + 1;
  const y = (v: number) => height - 2 - (v / max) * (height - 4);

  // Javobsiz nuqtalar chiziqni uzadi — segmentlarga ajratamiz.
  const segs: string[] = [];
  let cur: string[] = [];
  data.forEach((v, i) => {
    if (v < 0) {
      if (cur.length) segs.push(cur.join(' '));
      cur = [];
    } else {
      cur.push(`${cur.length ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`);
    }
  });
  if (cur.length) segs.push(cur.join(' '));

  const misses = data.map((v, i) => (v < 0 ? i : -1)).filter((i) => i >= 0);

  return (
    <svg width={width} height={height} className="overflow-visible" aria-hidden>
      {segs.map((d, i) => (
        <path key={i} d={d} fill="none" stroke="#3987e5" strokeWidth={1.5} strokeLinejoin="round" />
      ))}
      {misses.map((i) => (
        <circle key={i} cx={x(i)} cy={height - 3} r={1.8} fill="#d03b3b" />
      ))}
    </svg>
  );
}
