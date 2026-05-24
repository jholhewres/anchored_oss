import React from "react";

interface IcProps {
  d: React.ReactNode;
  size?: number;
  sw?: number;
  fill?: string;
}

const Ic: React.FC<IcProps> = ({ d, size = 16, sw = 1.6, fill }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill={fill || "none"}
    stroke="currentColor"
    strokeWidth={sw}
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {d}
  </svg>
);

type IconProps = { size?: number; sw?: number; className?: string };

const makeIcon = (d: React.ReactNode): React.FC<IconProps> => {
  const Icon: React.FC<IconProps> = (p) => <Ic d={d} {...p} />;
  Icon.displayName = "Icon";
  return Icon;
};

export const I: Record<string, React.FC<IconProps>> = {
  home: makeIcon(
    <>
      <path d="M3 10l9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1z" />
    </>
  ),
  layers: makeIcon(
    <>
      <path d="M12 2l9 5-9 5-9-5 9-5z" />
      <path d="M3 12l9 5 9-5" />
      <path d="M3 17l9 5 9-5" />
    </>
  ),
  folder: makeIcon(
    <>
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </>
  ),
  users: makeIcon(
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2 20c0-3 3-5 7-5s7 2 7 5" />
      <circle cx="17" cy="7" r="2.5" />
      <path d="M22 18c0-2.5-1.5-4-4-4" />
    </>
  ),
  user: makeIcon(
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21c0-4 4-7 8-7s8 3 8 7" />
    </>
  ),
  key: makeIcon(
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="M11 12l9-9" />
      <path d="M16 7l3 3" />
      <path d="M18 5l3 3" />
    </>
  ),
  shield: makeIcon(
    <>
      <path d="M12 2l8 3v7c0 5-4 9-8 10-4-1-8-5-8-10V5z" />
    </>
  ),
  activity: makeIcon(
    <>
      <path d="M3 12h4l3-9 4 18 3-9h4" />
    </>
  ),
  pulse: makeIcon(
    <>
      <path d="M3 12h3l2-3 4 6 3-9 2 6h4" />
    </>
  ),
  database: makeIcon(
    <>
      <ellipse cx="12" cy="5" rx="9" ry="3" />
      <path d="M3 5v6c0 1.7 4 3 9 3s9-1.3 9-3V5" />
      <path d="M3 11v6c0 1.7 4 3 9 3s9-1.3 9-3v-6" />
    </>
  ),
  graph: makeIcon(
    <>
      <circle cx="6" cy="6" r="2.5" />
      <circle cx="18" cy="6" r="2.5" />
      <circle cx="6" cy="18" r="2.5" />
      <circle cx="18" cy="18" r="2.5" />
      <circle cx="12" cy="12" r="2.5" />
      <path d="M8 7l2.5 4M16 7l-2.5 4M13.5 13l3 3.5M10.5 13l-3 3.5" />
    </>
  ),
  brain: makeIcon(
    <>
      <path d="M9 3a3 3 0 0 0-3 3v1a3 3 0 0 0-2 5 3 3 0 0 0 2 5v1a3 3 0 0 0 3 3h1V3z" />
      <path d="M14 3h1a3 3 0 0 1 3 3v1a3 3 0 0 1 2 5 3 3 0 0 1-2 5v1a3 3 0 0 1-3 3h-1V3z" />
    </>
  ),
  search: makeIcon(
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-4-4" />
    </>
  ),
  filter: makeIcon(
    <>
      <path d="M3 5h18l-7 9v6l-4-2v-4z" />
    </>
  ),
  plus: makeIcon(
    <>
      <path d="M12 5v14M5 12h14" />
    </>
  ),
  minus: makeIcon(
    <>
      <path d="M5 12h14" />
    </>
  ),
  x: makeIcon(
    <>
      <path d="M6 6l12 12M18 6L6 18" />
    </>
  ),
  check: makeIcon(
    <>
      <path d="M4 12l5 5L20 6" />
    </>
  ),
  chevR: makeIcon(
    <>
      <path d="M9 6l6 6-6 6" />
    </>
  ),
  chevD: makeIcon(
    <>
      <path d="M6 9l6 6 6-6" />
    </>
  ),
  chevU: makeIcon(
    <>
      <path d="M6 15l6-6 6 6" />
    </>
  ),
  chevL: makeIcon(
    <>
      <path d="M15 6l-6 6 6 6" />
    </>
  ),
  arrowR: makeIcon(
    <>
      <path d="M5 12h14M13 5l7 7-7 7" />
    </>
  ),
  arrowUR: makeIcon(
    <>
      <path d="M7 17L17 7M8 7h9v9" />
    </>
  ),
  copy: makeIcon(
    <>
      <rect x="8" y="8" width="13" height="13" rx="2" />
      <path d="M16 8V4a1 1 0 0 0-1-1H4a1 1 0 0 0-1 1v11a1 1 0 0 0 1 1h4" />
    </>
  ),
  external: makeIcon(
    <>
      <path d="M14 4h6v6M20 4L11 13M19 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" />
    </>
  ),
  more: makeIcon(
    <>
      <circle cx="5" cy="12" r="1.5" fill="currentColor" stroke="none" />
      <circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none" />
      <circle cx="19" cy="12" r="1.5" fill="currentColor" stroke="none" />
    </>
  ),
  settings: makeIcon(
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1" />
    </>
  ),
  download: makeIcon(
    <>
      <path d="M12 3v12M6 11l6 6 6-6M4 21h16" />
    </>
  ),
  upload: makeIcon(
    <>
      <path d="M12 21V9M6 13l6-6 6 6M4 3h16" />
    </>
  ),
  bell: makeIcon(
    <>
      <path d="M6 9a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9z" />
      <path d="M10 21a2 2 0 0 0 4 0" />
    </>
  ),
  bolt: makeIcon(
    <>
      <path d="M13 3L4 14h7l-1 7 9-11h-7z" />
    </>
  ),
  cube: makeIcon(
    <>
      <path d="M12 2L3 7v10l9 5 9-5V7z" />
      <path d="M3 7l9 5 9-5M12 12v10" />
    </>
  ),
  link: makeIcon(
    <>
      <path d="M10 14a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1" />
      <path d="M14 10a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1" />
    </>
  ),
  unlock: makeIcon(
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 9.9-1" />
    </>
  ),
  lock: makeIcon(
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  eye: makeIcon(
    <>
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  eyeOff: makeIcon(
    <>
      <path d="M17 17a9 9 0 0 1-5 2C6 19 2 12 2 12a18 18 0 0 1 4-5" />
      <path d="M9 5a9 9 0 0 1 3-1c6 0 10 7 10 7a18 18 0 0 1-2 3" />
      <path d="M14 10a3 3 0 1 1-4 4M2 2l20 20" />
    </>
  ),
  refresh: makeIcon(
    <>
      <path d="M21 12a9 9 0 0 0-9-9 9 9 0 0 0-6 2.3L3 8" />
      <path d="M3 3v5h5" />
      <path d="M3 12a9 9 0 0 0 9 9 9 9 0 0 0 6-2.3L21 16" />
      <path d="M21 21v-5h-5" />
    </>
  ),
  sun: makeIcon(
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M2 12h2M20 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4" />
    </>
  ),
  moon: makeIcon(
    <>
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
    </>
  ),
  github: makeIcon(
    <>
      <path d="M9 19c-5 1.5-5-2.5-7-3M15 21v-3a2.8 2.8 0 0 0-.8-2c2.8-.3 5.8-1.4 5.8-6a4.7 4.7 0 0 0-1.3-3.3 4.4 4.4 0 0 0-.1-3.2s-1.1-.3-3.5 1.3a12 12 0 0 0-6.4 0c-2.4-1.6-3.5-1.3-3.5-1.3a4.4 4.4 0 0 0-.1 3.2A4.7 4.7 0 0 0 3.8 9c0 4.5 3 5.7 5.7 6A2.8 2.8 0 0 0 9 17v4" />
    </>
  ),
  terminal: makeIcon(
    <>
      <polyline points="4 17 10 11 4 5" />
      <line x1="12" y1="19" x2="20" y2="19" />
    </>
  ),
  zap: makeIcon(
    <>
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </>
  ),
  clock: makeIcon(
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  trash: makeIcon(
    <>
      <path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
    </>
  ),
  edit: makeIcon(
    <>
      <path d="M11 4H4a1 1 0 0 0-1 1v15a1 1 0 0 0 1 1h15a1 1 0 0 0 1-1v-7" />
      <path d="M18.5 2.5a2.1 2.1 0 1 1 3 3L12 15l-4 1 1-4z" />
    </>
  ),
  inbox: makeIcon(
    <>
      <path d="M22 12h-6l-2 3h-4l-2-3H2" />
      <path d="M5 4l-3 8v7a1 1 0 0 0 1 1h18a1 1 0 0 0 1-1v-7l-3-8a1 1 0 0 0-1-1H6a1 1 0 0 0-1 1z" />
    </>
  ),
  globe: makeIcon(
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z" />
    </>
  ),
  branch: makeIcon(
    <>
      <circle cx="6" cy="3" r="2" />
      <circle cx="6" cy="21" r="2" />
      <circle cx="18" cy="6" r="2" />
      <path d="M6 5v14M18 8a6 6 0 0 1-6 6h-2" />
    </>
  ),
  tag: makeIcon(
    <>
      <path d="M20 12l-8 8L3 11V3h8z" />
      <circle cx="7" cy="7" r="1.2" fill="currentColor" />
    </>
  ),
  star: makeIcon(
    <>
      <polygon points="12 2 15 9 22 9 17 14 19 21 12 17 5 21 7 14 2 9 9 9 12 2" />
    </>
  ),
  alert: makeIcon(
    <>
      <path d="M10.3 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </>
  ),
  info: makeIcon(
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 16v-4M12 8h.01" />
    </>
  ),
  fileText: makeIcon(
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
      <path d="M14 2v6h6M8 13h8M8 17h5M8 9h2" />
    </>
  ),
};

interface AnchoredLogoProps {
  size?: number;
  color?: string;
  accent?: string;
  wordmark?: boolean;
  label?: string;
  mono?: boolean;
}

export const AnchoredLogo: React.FC<AnchoredLogoProps> = ({
  size = 22,
  color,
  accent,
  wordmark = true,
  label = "anchored",
  mono = false,
}) => {
  const fg = color || "currentColor";
  const ac = accent || "var(--accent)";

  const cube = (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      style={{ flex: "none" }}
    >
      <g>
        <path d="M12 2 L21 7 L12 12 L3 7 Z" fill={fg} opacity="0.92" />
        <path d="M3 7 L3 17 L12 22 L12 12 Z" fill={fg} opacity="0.55" />
        <path d="M21 7 L21 17 L12 22 L12 12 Z" fill={ac} />
        <path
          d="M12 2 L12 12 M3 7 L12 12 L21 7"
          fill="none"
          stroke="var(--bg)"
          strokeWidth="0.6"
          opacity="0.4"
        />
      </g>
    </svg>
  );

  if (!wordmark) return cube;

  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 8,
        color: fg,
      }}
    >
      {cube}
      <span
        style={{
          fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)",
          fontWeight: 600,
          fontSize: size * 0.78,
          letterSpacing: mono ? 0 : -0.3,
          lineHeight: 1,
        }}
      >
        {label}
      </span>
    </div>
  );
};

interface AnchoredOSSLogoProps {
  size?: number;
  mono?: boolean;
}

export const AnchoredOSSLogo: React.FC<AnchoredOSSLogoProps> = ({
  size = 22,
  mono = false,
}) => {
  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 8,
        color: "currentColor",
      }}
    >
      <AnchoredLogo size={size} wordmark={false} />
      <span
        style={{
          fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)",
          fontWeight: 600,
          fontSize: size * 0.78,
          letterSpacing: mono ? 0 : -0.3,
          lineHeight: 1,
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
        }}
      >
        anchored
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontWeight: 500,
            fontSize: size * 0.55,
            padding: "2px 6px",
            background: "var(--accent-bg)",
            color: "var(--accent)",
            border: "1px solid var(--accent-border)",
            borderRadius: 4,
            letterSpacing: 0.5,
            textTransform: "uppercase",
          }}
        >
          oss
        </span>
      </span>
    </div>
  );
};
