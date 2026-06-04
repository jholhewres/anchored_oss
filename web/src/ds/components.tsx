import React, { useState } from "react";
import { I } from "./icons";

type IconElement = React.ReactElement<React.SVGProps<SVGSVGElement> & { size?: number }>;

interface BtnProps extends React.HTMLAttributes<HTMLElement> {
  variant?: "default" | "primary" | "ghost" | "outline" | "danger" | "accent";
  size?: "sm" | "md" | "lg";
  icon?: IconElement;
  iconR?: IconElement;
  full?: boolean;
  as?: React.ElementType;
  href?: string;
  target?: string;
  rel?: string;
  type?: "button" | "submit" | "reset";
  disabled?: boolean;
}

export const Btn: React.FC<BtnProps> = ({
  children,
  variant = "default",
  size = "md",
  icon,
  iconR,
  full,
  as: Tag = "button",
  href,
  style = {},
  ...rest
}) => {
  const sizes = {
    sm: { h: 28, px: 10, fs: 12, gap: 6, ic: 13 },
    md: { h: 34, px: 14, fs: 13.5, gap: 7, ic: 15 },
    lg: { h: 42, px: 18, fs: 15, gap: 8, ic: 16 },
  };
  const s = sizes[size];
  const variants = {
    default: { bg: "var(--bg-2)", color: "var(--text)", border: "var(--border)", hover: "var(--bg-3)" },
    primary: { bg: "var(--accent)", color: "#fff", border: "transparent", hover: "var(--accent-hover)" },
    ghost: { bg: "transparent", color: "var(--text)", border: "transparent", hover: "var(--bg-2)" },
    outline: { bg: "transparent", color: "var(--text)", border: "var(--border-strong, var(--border))", hover: "var(--bg-2)" },
    danger: { bg: "transparent", color: "var(--err)", border: "var(--border)", hover: "var(--err-bg)" },
    accent: { bg: "var(--accent-bg)", color: "var(--accent)", border: "var(--accent-border)", hover: "color-mix(in srgb, var(--accent) 14%, transparent)" },
  };
  const v = variants[variant] || variants.default;
  const props = href ? { href } : {};
  return (
    <Tag
      {...props}
      {...rest}
      className={"a-btn " + (rest.className || "")}
      style={{
        display: full ? "flex" : "inline-flex",
        width: full ? "100%" : undefined,
        alignItems: "center",
        justifyContent: "center",
        gap: s.gap,
        height: s.h,
        padding: `0 ${s.px}px`,
        fontSize: s.fs,
        fontWeight: 500,
        background: v.bg,
        color: v.color,
        border: `1px solid ${v.border}`,
        borderRadius: "var(--radius)",
        cursor: "pointer",
        whiteSpace: "nowrap",
        letterSpacing: -0.1,
        transition: "background .12s, border-color .12s, color .12s",
        textDecoration: "none",
        ...style,
      }}
      onMouseEnter={(e: React.MouseEvent<HTMLElement>) => {
        (e.currentTarget as HTMLElement).style.background = v.hover;
      }}
      onMouseLeave={(e: React.MouseEvent<HTMLElement>) => {
        (e.currentTarget as HTMLElement).style.background = v.bg;
      }}
    >
      {icon && (
        <span style={{ display: "inline-flex" }}>
          {React.cloneElement(icon, { size: s.ic })}
        </span>
      )}
      {children}
      {iconR && (
        <span style={{ display: "inline-flex" }}>
          {React.cloneElement(iconR, { size: s.ic })}
        </span>
      )}
    </Tag>
  );
};

interface InputProps {
  icon?: IconElement;
  prefix?: string;
  suffix?: string;
  mono?: boolean;
  error?: boolean;
  full?: boolean;
  size?: "sm" | "md" | "lg";
  style?: React.CSSProperties;
  className?: string;
  onFocus?: React.FocusEventHandler<HTMLInputElement>;
  onBlur?: React.FocusEventHandler<HTMLInputElement>;
  onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
  placeholder?: string;
  value?: string;
  defaultValue?: string;
  onChange?: React.ChangeEventHandler<HTMLInputElement>;
  type?: string;
  disabled?: boolean;
  readOnly?: boolean;
  name?: string;
  id?: string;
  autoFocus?: boolean;
  autoComplete?: string;
  maxLength?: number;
  minLength?: number;
  required?: boolean;
  pattern?: string;
  inputMode?: React.InputHTMLAttributes<HTMLInputElement>["inputMode"];
  tabIndex?: number;
  "aria-label"?: string;
  "aria-describedby"?: string;
}

export const Input: React.FC<InputProps> = ({
  icon,
  prefix,
  suffix,
  mono,
  error,
  full,
  size = "md",
  style = {},
  onFocus,
  onBlur,
  onKeyDown,
  className,
  placeholder,
  value,
  defaultValue,
  onChange,
  type,
  disabled,
  readOnly,
  name,
  id,
  autoFocus,
  autoComplete,
  maxLength,
  minLength,
  required,
  pattern,
  inputMode,
  tabIndex,
  "aria-label": ariaLabel,
  "aria-describedby": ariaDescribedby,
}) => {
  const sizes = {
    sm: { h: 28, px: 8, fs: 12 },
    md: { h: 34, px: 10, fs: 13.5 },
    lg: { h: 42, px: 14, fs: 15 },
  };
  const s = sizes[size];
  const [focused, setFocused] = useState(false);
  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        width: full ? "100%" : undefined,
        height: s.h,
        padding: `0 ${s.px}px`,
        background: "var(--bg-input)",
        border: `1px solid ${error ? "var(--err)" : focused ? "var(--accent)" : "var(--border)"}`,
        borderRadius: "var(--radius)",
        transition: "border-color .12s, box-shadow .12s",
        boxShadow: focused
          ? "0 0 0 3px color-mix(in srgb, var(--accent) 16%, transparent)"
          : "none",
        gap: 6,
        ...style,
      }}
    >
      {icon && (
        <span style={{ color: "var(--text-dim)", display: "inline-flex" }}>
          {React.cloneElement(icon, { size: 14 })}
        </span>
      )}
      {prefix && (
        <span
          style={{
            color: "var(--text-dim)",
            fontFamily: "var(--font-mono)",
            fontSize: s.fs * 0.95,
          }}
        >
          {prefix}
        </span>
      )}
      <input
        className={className}
        placeholder={placeholder}
        value={value}
        defaultValue={defaultValue}
        onChange={onChange}
        onKeyDown={onKeyDown}
        type={type}
        disabled={disabled}
        readOnly={readOnly}
        name={name}
        id={id}
        autoFocus={autoFocus}
        autoComplete={autoComplete}
        maxLength={maxLength}
        minLength={minLength}
        required={required}
        pattern={pattern}
        inputMode={inputMode}
        tabIndex={tabIndex}
        aria-label={ariaLabel}
        aria-describedby={ariaDescribedby}
        onFocus={(e) => {
          setFocused(true);
          onFocus?.(e);
        }}
        onBlur={(e) => {
          setFocused(false);
          onBlur?.(e);
        }}
        style={{
          flex: 1,
          minWidth: 0,
          height: "100%",
          border: 0,
          outline: 0,
          background: "transparent",
          color: "var(--text)",
          fontSize: s.fs,
          fontFamily: mono ? "var(--font-mono)" : "inherit",
        }}
      />
      {suffix && (
        <span
          style={{
            color: "var(--text-dim)",
            fontFamily: "var(--font-mono)",
            fontSize: s.fs * 0.9,
          }}
        >
          {suffix}
        </span>
      )}
    </div>
  );
};

interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  inset?: boolean;
}

export const Card: React.FC<CardProps> = ({
  children,
  style = {},
  inset,
  ...rest
}) => {
  return (
    <div
      {...rest}
      style={{
        background: inset ? "var(--bg-1)" : "var(--bg-2)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
        overflow: "hidden",
        ...style,
      }}
    >
      {children}
    </div>
  );
};

interface BadgeProps {
  children?: React.ReactNode;
  tone?: "neutral" | "accent" | "ok" | "warn" | "err" | "info" | "outline";
  icon?: IconElement;
  dot?: boolean;
  style?: React.CSSProperties;
}

export const Badge: React.FC<BadgeProps> = ({
  children,
  tone = "neutral",
  icon,
  dot,
  style = {},
}) => {
  const tones = {
    neutral: { bg: "var(--bg-3)", color: "var(--text-muted)", border: "var(--border)", dot: "var(--text-dim)" },
    accent: { bg: "var(--accent-bg)", color: "var(--accent)", border: "var(--accent-border)", dot: "var(--accent)" },
    ok: { bg: "var(--ok-bg)", color: "var(--ok)", border: "color-mix(in srgb, var(--ok) 25%, transparent)", dot: "var(--ok)" },
    warn: { bg: "var(--warn-bg)", color: "var(--warn)", border: "color-mix(in srgb, var(--warn) 25%, transparent)", dot: "var(--warn)" },
    err: { bg: "var(--err-bg)", color: "var(--err)", border: "color-mix(in srgb, var(--err) 25%, transparent)", dot: "var(--err)" },
    info: { bg: "var(--info-bg)", color: "var(--info)", border: "color-mix(in srgb, var(--info) 25%, transparent)", dot: "var(--info)" },
    outline: { bg: "transparent", color: "var(--text-muted)", border: "var(--border-strong)", dot: "var(--text-muted)" },
  };
  const t = tones[tone] || tones.neutral;
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        padding: "2px 7px",
        fontSize: 11,
        fontWeight: 500,
        fontFamily: "var(--font-mono)",
        background: t.bg,
        color: t.color,
        border: `1px solid ${t.border}`,
        borderRadius: 4,
        height: 19,
        letterSpacing: 0.1,
        whiteSpace: "nowrap",
        ...style,
      }}
    >
      {dot && (
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: 3,
            background: t.dot,
            flex: "none",
            boxShadow: `0 0 0 2px color-mix(in srgb, ${t.dot} 18%, transparent)`,
          }}
        />
      )}
      {icon && React.cloneElement(icon, { size: 11 })}
      {children}
    </span>
  );
};

interface ScopeChipProps {
  scope: string;
}

export const ScopeChip: React.FC<ScopeChipProps> = ({ scope }) => {
  const map: Record<string, { tone: BadgeProps["tone"]; label: string }> = {
    admin: { tone: "err", label: "admin" },
    sync: { tone: "accent", label: "sync" },
    readonly: { tone: "info", label: "readonly" },
    write: { tone: "warn", label: "write" },
  };
  const v = map[scope] || { tone: "neutral" as const, label: scope };
  return <Badge tone={v.tone}>{v.label}</Badge>;
};

interface StatusProps {
  value?: string;
  label?: string;
  mono?: boolean;
}

export const Status: React.FC<StatusProps> = ({
  value = "ok",
  label,
  mono = true,
}) => {
  const map: Record<string, string> = {
    ok: "var(--ok)",
    online: "var(--ok)",
    healthy: "var(--ok)",
    syncing: "var(--info)",
    pending: "var(--info)",
    warn: "var(--warn)",
    degraded: "var(--warn)",
    err: "var(--err)",
    offline: "var(--err)",
    failed: "var(--err)",
    dim: "var(--text-dim)",
  };
  const c = map[value] || "var(--text-dim)";
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        fontFamily: mono ? "var(--font-mono)" : "inherit",
        fontSize: 12,
        color: "var(--text-muted)",
      }}
    >
      <span
        style={{
          position: "relative",
          width: 7,
          height: 7,
          borderRadius: 4,
          background: c,
          flex: "none",
          boxShadow: `0 0 0 3px color-mix(in srgb, ${c} 18%, transparent)`,
        }}
      >
        {(value === "syncing" || value === "pending") && (
          <span
            style={{
              position: "absolute",
              inset: -4,
              borderRadius: 8,
              border: `1.5px solid ${c}`,
              opacity: 0.45,
              animation: "a-pulse 1.6s ease-out infinite",
            }}
          />
        )}
      </span>
      {label || value}
    </span>
  );
};

interface CodeProps {
  children?: React.ReactNode;
  lang?: string;
  copy?: boolean;
  prompt?: string;
  lines?: (string | React.ReactNode)[];
  style?: React.CSSProperties;
  scrollable?: boolean;
}

export const Code: React.FC<CodeProps> = ({
  children,
  lang,
  copy = true,
  prompt,
  lines,
  style = {},
  scrollable = false,
}) => {
  const lineArr =
    lines ||
    (typeof children === "string" ? children.split("\n") : null);
  return (
    <div
      style={{
        background: "var(--bg-1)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius)",
        fontFamily: "var(--font-mono)",
        fontSize: 12.5,
        lineHeight: 1.7,
        overflow: "hidden",
        ...style,
      }}
    >
      {(lang || copy) && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "6px 10px",
            borderBottom: "1px solid var(--border)",
            background: "var(--bg-2)",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              color: "var(--text-dim)",
              fontSize: 11,
            }}
          >
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: 4,
                background: "var(--text-ghost)",
              }}
            />
            {lang && <span>{lang}</span>}
          </div>
          {copy && (
            <button
              style={{
                border: 0,
                background: "transparent",
                color: "var(--text-dim)",
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                cursor: "pointer",
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                padding: 2,
              }}
            >
              <I.copy size={12} />
            </button>
          )}
        </div>
      )}
      <div
        style={{
          padding: "12px 14px",
          color: "var(--text)",
          overflowX: scrollable ? "auto" : "visible",
          whiteSpace: "pre",
        }}
      >
        {lineArr
          ? lineArr.map((l, i) => (
              <div key={i} style={{ display: "flex", gap: 12 }}>
                {prompt && (
                  <span
                    style={{
                      color: "var(--text-ghost)",
                      flex: "none",
                      userSelect: "none",
                    }}
                  >
                    {prompt}
                  </span>
                )}
                <span
                  dangerouslySetInnerHTML={
                    typeof l === "string" ? { __html: l } : undefined
                  }
                >
                  {typeof l !== "string" ? l : undefined}
                </span>
              </div>
            ))
          : children}
      </div>
    </div>
  );
};

interface InstallCmdProps {
  cmd?: string;
  label?: string;
  accent?: boolean;
}

export const InstallCmd: React.FC<InstallCmdProps> = ({
  cmd = "curl -fsSL https://raw.githubusercontent.com/jholhewres/anchored/main/install/install.sh | bash",
  accent = false,
}) => {
  const [copied, setCopied] = useState(false);

  const copyCommand = async () => {
    try {
      await navigator.clipboard.writeText(cmd);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = cmd;
      textarea.setAttribute("readonly", "");
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 0,
        background: accent ? "var(--bg-1)" : "var(--bg-2)",
        border: `1px solid ${accent ? "var(--accent-border)" : "var(--border)"}`,
        borderRadius: "var(--radius)",
        fontFamily: "var(--font-mono)",
        fontSize: 13,
        padding: 0,
        maxWidth: "100%",
      }}
    >
      <span
        style={{
          padding: "10px 0 10px 14px",
          color: "var(--text-dim)",
          userSelect: "none",
        }}
      >
        $
      </span>
      <span
        style={{
          padding: "10px 14px",
          color: "var(--text)",
          flex: 1,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {cmd}
      </span>
      <button
        type="button"
        aria-label={copied ? "Install command copied" : "Copy install command"}
        onClick={copyCommand}
        style={{
          border: 0,
          borderLeft: "1px solid var(--border)",
          background: "transparent",
          color: copied ? "var(--ok)" : "var(--text-muted)",
          padding: "10px 12px",
          cursor: "pointer",
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          fontFamily: "inherit",
          fontSize: 12,
          minWidth: 78,
          justifyContent: "center",
        }}
      >
        {copied ? <I.check size={14} /> : <I.copy size={14} />}
        {copied ? "copied" : "copy"}
      </button>
    </div>
  );
};

interface SectionLabelProps {
  n: string | number;
  label: string;
  kicker?: string;
  style?: React.CSSProperties;
}

export const SectionLabel: React.FC<SectionLabelProps> = ({
  n,
  label,
  kicker,
  style = {},
}) => {
  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 10,
        fontFamily: "var(--font-mono)",
        fontSize: 12,
        color: "var(--text-dim)",
        letterSpacing: 0.5,
        textTransform: "uppercase",
        ...style,
      }}
    >
      <span style={{ color: "var(--accent)" }}>[{n}]</span>
      <span
        style={{ width: 24, height: 1, background: "var(--border-strong)" }}
      />
      <span>{label}</span>
      {kicker && <span style={{ color: "var(--text-ghost)" }}>{kicker}</span>}
    </div>
  );
};

interface MetricProps {
  label: string;
  value: string | number;
  unit?: string;
  delta?: string;
  trend?: "up" | "down" | "flat";
  icon?: IconElement;
  sub?: React.ReactNode;
  accent?: boolean;
}

export const Metric: React.FC<MetricProps> = ({
  label,
  value,
  unit,
  delta,
  trend = "flat",
  icon,
  sub,
  accent,
}) => {
  const deltaColor =
    trend === "up"
      ? "var(--ok)"
      : trend === "down"
        ? "var(--err)"
        : "var(--text-dim)";
  return (
    <Card style={{ padding: 16 }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 12,
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            color: "var(--text-muted)",
            fontSize: 12,
            fontFamily: "var(--font-mono)",
            letterSpacing: 0.2,
          }}
        >
          {icon && React.cloneElement(icon, { size: 13 })}
          {label}
        </div>
        {delta && (
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 11,
              color: deltaColor,
            }}
          >
            {trend === "up" ? "\u2191" : trend === "down" ? "\u2193" : "\u00b7"}{" "}
            {delta}
          </span>
        )}
      </div>
      <div
        style={{ display: "flex", alignItems: "baseline", gap: 6 }}
      >
        <span
          style={{
            fontSize: 30,
            fontWeight: 500,
            letterSpacing: -0.8,
            color: accent ? "var(--accent)" : "var(--text)",
            fontFeatureSettings: '"tnum"',
          }}
        >
          {value}
        </span>
        {unit && (
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 13,
              color: "var(--text-dim)",
            }}
          >
            {unit}
          </span>
        )}
      </div>
      {sub && (
        <div style={{ marginTop: 10, fontSize: 12, color: "var(--text-dim)" }}>
          {sub}
        </div>
      )}
    </Card>
  );
};

interface ColDef {
  key: string;
  label: string;
  align?: "left" | "center" | "right";
  w?: string | number;
  muted?: boolean;
  mono?: boolean;
}

interface TableProps {
  cols: ColDef[];
  rows: Record<string, React.ReactNode>[];
  dense?: boolean;
}

export const Table: React.FC<TableProps> = ({ cols, rows, dense = false }) => {
  return (
    <div
      style={{
        overflow: "hidden",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
      }}
    >
      <table
        style={{
          width: "100%",
          borderCollapse: "collapse",
          fontSize: 13.5,
        }}
      >
        <thead>
          <tr style={{ background: "var(--bg-1)" }}>
            {cols.map((c, i) => (
              <th
                key={i}
                style={{
                  textAlign: c.align || "left",
                  padding: dense ? "8px 12px" : "12px 14px",
                  color: "var(--text-dim)",
                  fontWeight: 500,
                  fontSize: 11,
                  letterSpacing: 0.4,
                  textTransform: "uppercase",
                  fontFamily: "var(--font-mono)",
                  borderBottom: "1px solid var(--border)",
                  width: c.w,
                }}
              >
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, ri) => (
            <tr
              key={ri}
              style={{
                borderBottom:
                  ri < rows.length - 1 ? "1px solid var(--border)" : "none",
              }}
            >
              {cols.map((c, ci) => (
                <td
                  key={ci}
                  style={{
                    textAlign: c.align || "left",
                    padding: dense ? "8px 12px" : "12px 14px",
                    color: c.muted ? "var(--text-muted)" : "var(--text)",
                    fontFamily: c.mono ? "var(--font-mono)" : "inherit",
                    fontSize: c.mono ? 12.5 : 13.5,
                    verticalAlign: "middle",
                  }}
                >
                  {r[c.key]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

interface EmptyProps {
  icon?: IconElement;
  title: string;
  body: React.ReactNode;
  actions?: React.ReactNode;
}

export const Empty: React.FC<EmptyProps> = ({ icon, title, body, actions }) => {
  return (
    <div
      style={{
        padding: "60px 32px",
        textAlign: "center",
        border: "1px dashed var(--border-strong)",
        borderRadius: "var(--radius-lg)",
        background: "var(--bg-1)",
      }}
    >
      <div
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          width: 48,
          height: 48,
          borderRadius: 12,
          background: "var(--bg-3)",
          color: "var(--text-muted)",
          border: "1px solid var(--border)",
          margin: "0 auto 16px",
        }}
      >
        {icon && React.cloneElement(icon, { size: 22 })}
      </div>
      <div style={{ fontSize: 16, fontWeight: 500, marginBottom: 6 }}>
        {title}
      </div>
      <div
        style={{
          fontSize: 13.5,
          color: "var(--text-muted)",
          maxWidth: 380,
          margin: "0 auto 18px",
          lineHeight: 1.55,
        }}
      >
        {body}
      </div>
      {actions && (
        <div style={{ display: "flex", gap: 8, justifyContent: "center" }}>
          {actions}
        </div>
      )}
    </div>
  );
};

interface KbdProps {
  children: React.ReactNode;
}

export const Kbd: React.FC<KbdProps> = ({ children }) => {
  return (
    <kbd
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        minWidth: 18,
        height: 18,
        padding: "0 4px",
        fontFamily: "var(--font-mono)",
        fontSize: 10.5,
        fontWeight: 500,
        background: "var(--bg-3)",
        color: "var(--text-muted)",
        border: "1px solid var(--border)",
        borderBottom: "1.5px solid var(--border-strong)",
        borderRadius: 4,
      }}
    >
      {children}
    </kbd>
  );
};

interface AvatarProps {
  name?: string;
  size?: number;
  color?: string;
}

export const Avatar: React.FC<AvatarProps> = ({
  name = "",
  size = 28,
  color,
}) => {
  const initials = name
    .split(/\s+/)
    .slice(0, 2)
    .map((s) => s[0] || "")
    .join("")
    .toUpperCase();
  const palette = [
    "#c2410c", "#7c2d12", "#365314", "#854d0e",
    "#0e7490", "#5b21b6", "#831843", "#9f1239",
  ];
  const hash = [...name].reduce((a, c) => a + c.charCodeAt(0), 0);
  const bg = color || palette[hash % palette.length];
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        width: size,
        height: size,
        borderRadius: size / 2,
        background: bg,
        color: "#fff",
        fontFamily: "var(--font-mono)",
        fontWeight: 600,
        fontSize: size * 0.4,
        letterSpacing: 0.3,
        flex: "none",
      }}
    >
      {initials || "\u00b7"}
    </span>
  );
};

interface TabItem {
  key: string;
  label: string;
  icon?: IconElement;
  count?: number;
}

interface TabsProps {
  tabs: TabItem[];
  active: string;
  onSet?: (key: string) => void;
}

export const Tabs: React.FC<TabsProps> = ({ tabs, active, onSet }) => {
  return (
    <div
      style={{
        display: "flex",
        gap: 0,
        borderBottom: "1px solid var(--border)",
      }}
    >
      {tabs.map((t) => {
        const on = t.key === active;
        return (
          <button
            key={t.key}
            onClick={() => onSet?.(t.key)}
            style={{
              border: 0,
              background: "transparent",
              padding: "10px 14px",
              fontFamily: "inherit",
              fontSize: 13.5,
              color: on ? "var(--text)" : "var(--text-muted)",
              fontWeight: 500,
              borderBottom: `2px solid ${on ? "var(--accent)" : "transparent"}`,
              marginBottom: -1,
              cursor: "pointer",
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            {t.icon && React.cloneElement(t.icon, { size: 14 })}
            {t.label}
            {t.count != null && (
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11,
                  color: "var(--text-dim)",
                }}
              >
                {t.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
};

interface AsciiRuleProps {
  char?: string;
  length?: number;
  color?: string;
}

export const AsciiRule: React.FC<AsciiRuleProps> = ({
  char = "\u2500",
  length = 60,
  color = "var(--text-ghost)",
}) => {
  return (
    <div
      style={{
        fontFamily: "var(--font-mono)",
        color,
        fontSize: 12,
        userSelect: "none",
        whiteSpace: "nowrap",
        overflow: "hidden",
      }}
    >
      {char.repeat(length)}
    </div>
  );
};
