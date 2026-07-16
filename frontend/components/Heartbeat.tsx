'use client';

// Heartbeat — logotip yonidagi ECG chizig'i. Chiziqning O'ZI qimirlamaydi;
// uning ichida bitta nuqtacha to'lqin bo'ylab yuguradi:
//   live=true  → yashil nuqta
//   live=false → qizil nuqta
// Nuqta cx/cy to'lqin nuqtalariga sinxron animatsiya qilinadi (chiziq ustida yuradi).
const WAVE = 'M0 8 H6 L7 8 L8 2 L9 14 L10 8 H30 L31 8 L32 2 L33 14 L34 8 H48';

// To'lqin tugun nuqtalari (x,y) — nuqta shular bo'ylab harakatlanadi.
const XS = [0, 6, 7, 8, 9, 10, 30, 31, 32, 33, 34, 48];
const YS = [8, 8, 8, 2, 14, 8, 8, 8, 2, 14, 8, 8];
const KT = XS.map((x) => (x / 48).toFixed(4)).join(';');

export default function Heartbeat({ live }: { live: boolean }) {
  const label = live ? 'Ulangan — jonli' : 'Uzilgan';
  return (
    <svg
      viewBox="0 0 48 16"
      preserveAspectRatio="none"
      className="h-4 w-11 shrink-0 overflow-hidden"
      role="img"
      aria-label={label}
    >
      <title>{label}</title>
      {/* Statik ECG chizig'i */}
      <path
        d={WAVE}
        fill="none"
        stroke="currentColor"
        className="text-ink-muted/40"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
      {/* Chiziq bo'ylab yuguradigan nuqta */}
      <circle r="1.8" className={live ? 'fill-good' : 'fill-critical'}>
        <animate attributeName="cx" dur="2.2s" repeatCount="indefinite" keyTimes={KT} values={XS.join(';')} />
        <animate attributeName="cy" dur="2.2s" repeatCount="indefinite" keyTimes={KT} values={YS.join(';')} />
        <animate attributeName="opacity" dur="2.2s" repeatCount="indefinite" keyTimes="0;0.04;0.96;1" values="0;1;1;0" />
      </circle>
    </svg>
  );
}
