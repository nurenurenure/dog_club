// Line illustrations from the design system: stroke 2.4, round caps, ink by default.
interface Props {
  size?: number;
  color?: string;
}

export function Bone({ size = 96, color = 'var(--ink)' }: Props) {
  return (
    <svg width={size} height={size / 2} viewBox="0 0 96 48" aria-hidden="true" color={color}>
      <path
        d="M71.23 18 A9 9 0 1 1 84.12 24 A9 9 0 1 1 71.23 30 L24.77 30 A9 9 0 1 1 11.88 24 A9 9 0 1 1 24.77 18 Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function Ball({ size = 88, color = 'var(--ink)' }: Props) {
  return (
    <svg width={size} height={(size * 72) / 88} viewBox="0 0 88 72" aria-hidden="true">
      <g fill="none" stroke={color} strokeWidth="2.4" strokeLinecap="round">
        <circle cx="28" cy="40" r="20" />
        <path d="M12 30 Q28 40 22 59" />
        <path d="M44 50 Q28 40 34 21" />
        <path d="M50 18 L80 4 M54 28 L84 16 M54 38 L78 30" />
      </g>
    </svg>
  );
}
