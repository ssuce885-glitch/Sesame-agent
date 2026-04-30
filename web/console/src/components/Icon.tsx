interface IconProps {
  size?: number;
  color?: string;
  className?: string;
}

function svg(size: number, className: string | undefined, children: React.ReactNode) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      style={{ display: "inline-block", verticalAlign: "middle", flexShrink: 0 }}
    >
      {children}
    </svg>
  );
}

/* ─── Core ─────────────────────────────────────────────────────── */

export function MessageSquare({ size = 18, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10z" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function Activity({ size = 18, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function BarChart({ size = 18, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <line x1="18" y1="20" x2="18" y2="10" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="12" y1="20" x2="12" y2="4" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="6" y1="20" x2="6" y2="14" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
    </g>
  ));
}

export function Users({ size = 18, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <circle cx="9" cy="7" r="4" stroke={color} strokeWidth="1.8" fill="none" />
      <path d="M23 21v-2a4 4 0 0 0-3-3.87" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

/* ─── Navigation / UI ──────────────────────────────────────────── */

export function ChevronDown({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polyline points="6 9 12 15 18 9" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function ChevronUp({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polyline points="18 15 12 9 6 15" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function ChevronRight({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polyline points="9 18 15 12 9 6" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function ArrowDown({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <line x1="12" y1="5" x2="12" y2="19" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <polyline points="19 12 12 19 5 12" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Copy({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" stroke={color} strokeWidth="2" fill="none" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function RefreshCw({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <polyline points="23 4 23 10 17 10" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="1 20 1 14 7 14" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Terminal({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <polyline points="4 17 10 11 4 5" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <line x1="12" y1="19" x2="20" y2="19" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function Globe({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <circle cx="12" cy="12" r="10" stroke={color} strokeWidth="2" fill="none" />
      <line x1="2" y1="12" x2="22" y2="12" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" stroke={color} strokeWidth="2" fill="none" />
    </g>
  ));
}

export function Clock({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <circle cx="12" cy="12" r="10" stroke={color} strokeWidth="2" fill="none" />
      <polyline points="12 6 12 12 16 14" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Cpu({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <rect x="4" y="4" width="16" height="16" rx="2" ry="2" stroke={color} strokeWidth="2" fill="none" />
      <rect x="9" y="9" width="6" height="6" stroke={color} strokeWidth="2" fill="none" />
      <line x1="9" y1="1" x2="9" y2="4" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="15" y1="1" x2="15" y2="4" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="9" y1="20" x2="9" y2="23" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="15" y1="20" x2="15" y2="23" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="20" y1="9" x2="23" y2="9" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="20" y1="14" x2="23" y2="14" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="1" y1="9" x2="4" y2="9" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="1" y1="14" x2="4" y2="14" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function FileText({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="14 2 14 8 20 8" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <line x1="16" y1="13" x2="8" y2="13" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="16" y1="17" x2="8" y2="17" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <polyline points="10 9 9 9 8 9" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Database({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <ellipse cx="12" cy="5" rx="9" ry="3" stroke={color} strokeWidth="2" fill="none" />
      <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Mail({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="22,6 12,13 2,6" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function X({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <path d="M18 6L6 18M6 6l12 12" stroke={color} strokeWidth="2" strokeLinecap="round" />
  ));
}

export function Check({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polyline points="20 6 9 17 4 12" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function Circle({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <circle cx="12" cy="12" r="5" stroke={color} strokeWidth="2" fill="none" />
  ));
}

export function Play({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <polygon points="5 3 19 12 5 21 5 3" fill={color} />
  ));
}

export function AlertTriangle({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <line x1="12" y1="9" x2="12" y2="13" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="12" y1="17" x2="12.01" y2="17" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function Wrench({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  ));
}

export function GitBranch({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <line x1="6" y1="3" x2="6" y2="15" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <circle cx="18" cy="6" r="3" stroke={color} strokeWidth="2" fill="none" />
      <circle cx="6" cy="18" r="3" stroke={color} strokeWidth="2" fill="none" />
      <path d="M18 9a9 9 0 0 1-9 9" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Layers({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <polygon points="12 2 2 7 12 12 22 7 12 2" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="2 17 12 22 22 17" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="2 12 12 17 22 12" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}

export function Trash({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <polyline points="3 6 5 6 21 6" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <line x1="10" y1="11" x2="10" y2="17" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="14" y1="11" x2="14" y2="17" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function Plus({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <line x1="12" y1="5" x2="12" y2="19" stroke={color} strokeWidth="2" strokeLinecap="round" />
      <line x1="5" y1="12" x2="19" y2="12" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function Eye({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <circle cx="12" cy="12" r="3" stroke={color} strokeWidth="2" fill="none" />
    </g>
  ));
}

export function EyeOff({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <line x1="1" y1="1" x2="23" y2="23" stroke={color} strokeWidth="2" strokeLinecap="round" />
    </g>
  ));
}

export function Save({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, className, (
    <g>
      <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="17 21 17 13 7 13 7 21" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <polyline points="7 3 7 8 15 8" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </g>
  ));
}
