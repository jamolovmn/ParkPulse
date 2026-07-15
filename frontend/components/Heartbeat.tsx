'use client';

// Heartbeat — ulanish holati ECG chizig'i sifatida.
//   live=true  → yashil "urib turadigan" chiziq (jonli monitor)
//   live=false → qizil tekis chiziq (yurak urishi yo'q)
//
// To'lqin ikki tildan iborat (kenglik 48), guruh -24 ga uzluksiz suriladi,
// shuning uchun 0..24 oynasida ular seamless takrorlanadi.
const WAVE =
  'M0 8 H6 L7 8 L8 2 L9 14 L10 8 H30 L31 8 L32 2 L33 14 L34 8 H48';

export default function Heartbeat({ live }: { live: boolean }) {
  return (
    <svg
      viewBox="0 0 24 16"
      preserveAspectRatio="none"
      className={`h-4 w-11 shrink-0 overflow-hidden ${live ? 'text-good' : 'text-critical'}`}
      role="img"
      aria-label={live ? 'Ulangan — jonli' : 'Uzilgan'}
    >
      <title>{live ? 'Ulangan — jonli' : 'Uzilgan'}</title>
      {live ? (
        <g className="pp-ecg-scroll">
          <path
            d={WAVE}
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            vectorEffect="non-scaling-stroke"
          />
        </g>
      ) : (
        <path
          d="M0 8 H24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
        />
      )}
    </svg>
  );
}
