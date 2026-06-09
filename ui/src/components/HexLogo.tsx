import { useId } from "react";

export type HexTone = "blue" | "red" | "gray";

const TONES: Record<HexTone, { field: string; ring: string }> = {
  blue: { field: "#2949c6", ring: "#4f74f0" },
  red: { field: "#c62a2a", ring: "#f05a5a" },
  gray: { field: "#64748b", ring: "#94a3b8" },
};

const HEX_POINTS = "16,2.8 27.43,9.4 27.43,22.6 16,29.2 4.57,22.6 4.57,9.4";

interface HexLogoProps {
  tone?: HexTone;
  className?: string;
  title?: string;
}

// The VideoNode brand mark: a hexagon badge with a chevron-and-node glyph.
// The field/ring hue encodes pipeline state (blue = running, red = stopped).
export function HexLogo({ tone = "blue", className, title }: Readonly<HexLogoProps>) {
  const clipId = useId();
  const { field, ring } = TONES[tone];
  return (
    <svg
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      role="img"
      aria-label={title}
    >
      {title ? <title>{title}</title> : null}
      <rect x="0" y="0" width="32" height="32" rx="6" fill={field} />
      <polygon points={HEX_POINTS} fill="#0a1329" />
      <polygon points={HEX_POINTS} fill="none" stroke={ring} strokeWidth="2" strokeLinejoin="miter" />
      <defs>
        <clipPath id={clipId}>
          <rect x="0" y="9.3" width="32" height="13.4" />
        </clipPath>
      </defs>
      <path
        d="M12 6.6 L21 16 L12 25.4"
        fill="none"
        stroke="#ffffff"
        strokeWidth="2.5"
        strokeLinejoin="miter"
        clipPath={`url(#${clipId})`}
      />
      <circle cx="11.6" cy="16" r="2.4" fill="#ffffff" />
    </svg>
  );
}
