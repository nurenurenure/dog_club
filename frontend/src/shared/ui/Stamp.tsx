import { useId } from 'react';

/** Club stamp: circular wordmark with a line bulldog muzzle. */
export function Stamp({ size = 48, color = 'var(--lime-light)' }: { size?: number; color?: string }) {
  const path = useId();
  return (
    <svg width={size} height={size} viewBox="0 0 120 120" aria-hidden="true">
      <defs>
        <path id={path} d="M60,60 m-42,0 a42,42 0 1,1 84,0 a42,42 0 1,1 -84,0" />
      </defs>
      <circle cx="60" cy="60" r="56" fill="none" stroke={color} strokeWidth="5" />
      <text fill={color} style={{ fontFamily: 'var(--font-display)', fontWeight: 800, fontSize: 14, letterSpacing: 2 }}>
        <textPath href={`#${path}`}>КЛУБ · СОБАКОВОДСТВА · КЛУБ · СОБАКОВОДСТВА ·</textPath>
      </text>
      <g transform="translate(40 40)" fill="none" stroke={color} strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
        <path d="M8 17 L3 3 L14 9" />
        <path d="M32 17 L37 3 L26 9" />
        <path d="M8 16 Q8 8 20 8 Q32 8 32 16 L32 25 Q32 36 20 36 Q8 36 8 25 Z" />
        <path d="M17.5 26 Q20 28.5 22.5 26 Q20 24.5 17.5 26 Z" />
      </g>
    </svg>
  );
}
