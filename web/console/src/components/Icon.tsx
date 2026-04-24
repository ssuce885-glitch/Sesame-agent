interface IconProps {
  size?: number;
  color?: string;
  className?: string;
}

function svg(size: number, color: string, className: string | undefined, children: React.ReactNode) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      style={{ display: "inline-block", verticalAlign: "middle", flexShrink: 0 }}
    >
      {children}
    </svg>
  );
}

export function Check({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M3.5 8.5L6.5 11.5L12.5 4.5" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  ));
}

export function X({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M4 4L12 12M12 4L4 12" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
  ));
}

export function ChevronDown({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M4 6L8 10L12 6" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  ));
}

export function ChevronUp({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M4 10L8 6L12 10" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  ));
}

export function Circle({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <circle cx="8" cy="8" r="4" stroke={color} strokeWidth="1.8" />
  ));
}

export function Play({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M5 3L13 8L5 13Z" fill={color} />
  ));
}

export function AlertTriangle({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <>
      <path d="M8 2L14.5 13.5H1.5L8 2Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" fill="none" />
      <path d="M8 6.5V9.5" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
      <circle cx="8" cy="11.5" r="0.8" fill={color} />
    </>
  ));
}

export function Shield({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M8 1.5L2.5 4V8C2.5 11.5 4.8 13.8 8 14.5C11.2 13.8 13.5 11.5 13.5 8V4L8 1.5Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" fill="none" />
  ));
}

export function ArrowDown({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M8 2V12M8 12L4 8M8 12L12 8" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  ));
}

export function Wrench({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M10.5 1.5L6 6L4.5 4.5L2 7L3.5 8.5L1.5 10.5L5.5 14.5L7.5 12.5L9 14L11.5 11.5L10 10L14.5 5.5C15.3 4.7 15.3 3.3 14.5 2.5L13.5 1.5C12.7 0.7 11.3 0.7 10.5 1.5Z" stroke={color} strokeWidth="1.3" strokeLinejoin="round" fill="none" />
  ));
}

export function MessageSquare({ size = 16, color = "currentColor", className }: IconProps) {
  return svg(size, color, className, (
    <path d="M2 3C2 2.44772 2.44772 2 3 2H13C13.5523 2 14 2.44772 14 3V10C14 10.5523 13.5523 11 13 11H5L2 14V3Z" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
  ));
}
