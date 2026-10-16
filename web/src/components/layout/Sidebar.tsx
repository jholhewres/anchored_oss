import React, { useEffect, useRef, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";

import { I, AnchoredLogo } from "@/ds/icons";
import { Avatar, ScopeChip } from "@/ds/components";
import { useAuth } from "@/lib/auth";
import { applyTheme, readTheme, THEME_MODES, type ThemeMode } from "@/lib/theme";
import { api } from "@/lib/api";
import type { Health } from "@/lib/types";

interface NavGroup {
  label?: string;
  items: {
    to: string;
    icon: React.ReactElement;
    label: string;
    count?: number;
    status?: string;
    end?: boolean;
    adminOnly?: boolean;
  }[];
}

const groups: NavGroup[] = [
  {
    items: [
      { to: "/dashboard", icon: <I.home />, label: "Overview" },
      { to: "/projects", icon: <I.folder />, label: "Projects" },
      { to: "/skills", icon: <I.layers />, label: "Skills", adminOnly: true },
      { to: "/my-api-keys", icon: <I.key />, label: "My API keys" },
      { to: "/developers", icon: <I.users />, label: "Developers", adminOnly: true },
    ],
  },
  {
    label: "Admin",
    items: [
      { to: "/guardrails", icon: <I.shield />, label: "Guardrails", adminOnly: true },
      { to: "/api-keys", icon: <I.key />, label: "Organization API keys", adminOnly: true },
      { to: "/audit", icon: <I.activity />, label: "Audit log", adminOnly: true },
      { to: "/health", icon: <I.pulse />, label: "Health" },
    ],
  },
];

export function Sidebar() {
  const { me, logout } = useAuth();
  const location = useLocation();
  const isAdmin = me?.scope === "admin";
  const [theme, setTheme] = useState<ThemeMode>(() => readTheme());

  const [dropOpen, setDropOpen] = useState(false);
  const [health, setHealth] = useState<Health | null>(null);
  const [healthLoaded, setHealthLoaded] = useState(false);
  const dropRef = useRef<HTMLDivElement>(null);

  // Fetch version once
  useEffect(() => {
    api.getHealth()
      .then((h) => { setHealth(h); setHealthLoaded(true); })
      .catch(() => { setHealthLoaded(true); });
  }, []);

  useEffect(() => {
    const sync = () => setTheme(readTheme());
    window.addEventListener("storage", sync);
    window.addEventListener("anchored-theme-change", sync as EventListener);
    return () => {
      window.removeEventListener("storage", sync);
      window.removeEventListener("anchored-theme-change", sync as EventListener);
    };
  }, []);

  // Click-outside to close dropdown
  useEffect(() => {
    if (!dropOpen) return;
    function handler(e: MouseEvent) {
      if (dropRef.current && !dropRef.current.contains(e.target as Node)) {
        setDropOpen(false);
      }
    }
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, [dropOpen]);

  function handleLogout() {
    logout();
    window.location.replace("/login");
  }

  function handleTheme(next: ThemeMode) {
    applyTheme(next);
    setTheme(next);
    setDropOpen(false);
  }

  return (
    <aside
      className="app-sidebar"
      style={{
        background: "var(--bg-1)",
        borderRight: "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        minWidth: 0,
        width: 240,
        flex: "none",
      }}
    >
      {/* Org pill with dropdown */}
      <div className="sidebar-org" style={{ padding: "16px 14px", borderBottom: "1px solid var(--border)", position: "relative" }} ref={dropRef}>
        <button
          className="sidebar-org-button"
          type="button"
          aria-label="Open organization menu"
          aria-expanded={dropOpen}
          aria-haspopup="menu"
          onClick={() => setDropOpen(o => !o)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "8px 10px",
            borderRadius: "var(--radius)",
            background: "var(--bg-2)",
            border: "1px solid var(--border)",
            width: "100%",
            cursor: "pointer",
            color: "inherit",
            fontFamily: "inherit",
          }}
        >
          <AnchoredLogo size={18} wordmark={false} />
          <div className="sidebar-org-copy" style={{ minWidth: 0, flex: 1, textAlign: "left" }}>
            <div style={{ fontSize: 13, fontWeight: 500, letterSpacing: -0.2 }}>
              anchored
            </div>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--text-dim)" }}>
              org · self-hosted
            </div>
          </div>
          <span className="sidebar-org-chevron"><I.chevD size={14} /></span>
        </button>

        {/* Dropdown */}
        {dropOpen && (
          <div className="sidebar-org-menu" style={{
            position: "absolute", top: "calc(100% - 2px)", left: 14, right: 14,
            background: "var(--bg-2)", border: "1px solid var(--border)",
            borderRadius: "var(--radius)", zIndex: 40,
            boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
            overflow: "hidden",
          }}>
            {/* Docs */}
            <a
              href="https://github.com/jholhewres/anchored_oss#readme"
              target="_blank"
              rel="noopener noreferrer"
              onClick={() => setDropOpen(false)}
              style={{
                display: "flex", alignItems: "center", gap: 9,
                padding: "10px 14px", fontSize: 13,
                color: "var(--text-muted)", textDecoration: "none",
                borderBottom: "1px solid var(--border)",
              }}
            >
              <I.external size={14} />
              Docs
            </a>

            {/* Version */}
            {health && (
              <div style={{
                padding: "8px 14px",
                fontFamily: "var(--font-mono)", fontSize: 11,
                color: "var(--text-dim)", borderBottom: "1px solid var(--border)",
              }}>
                {health.version}
              </div>
            )}

            <div style={{ padding: "12px 14px", borderBottom: "1px solid var(--border)" }}>
              <div
                className="sidebar-theme-label"
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10,
                  color: "var(--text-dim)",
                  letterSpacing: 0.5,
                  textTransform: "uppercase" as const,
                  marginBottom: 8,
                }}
              >
                Theme
              </div>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
                {THEME_MODES.map((option) => {
                  const on = theme === option;
                  return (
                    <button
                      key={option}
                      type="button"
                      aria-pressed={on}
                      onClick={() => handleTheme(option)}
                      style={{
                        borderRadius: "var(--radius-sm)",
                        border: on ? "1px solid var(--accent-border)" : "1px solid var(--border)",
                        background: on ? "var(--accent-bg)" : "var(--bg-1)",
                        color: on ? "var(--accent)" : "var(--text-muted)",
                        cursor: "pointer",
                        fontFamily: "var(--font-mono)",
                        fontSize: 11,
                        fontWeight: 500,
                        padding: "8px 10px",
                        textAlign: "left",
                      }}
                    >
                      {option === "light" ? "Light" : "Warm"}
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Logout */}
            <button
              type="button"
              onClick={handleLogout}
              style={{
                display: "flex", alignItems: "center", gap: 9,
                padding: "10px 14px", width: "100%",
                background: "transparent", border: 0,
                fontSize: 13, color: "var(--err)",
                cursor: "pointer", fontFamily: "inherit", textAlign: "left",
              }}
            >
              <I.arrowR size={14} />
              Log out
            </button>
          </div>
        )}
      </div>

      <nav
        className="sidebar-nav"
        style={{
          flex: 1,
          padding: "12px 10px",
          display: "flex",
          flexDirection: "column",
          gap: 16,
          overflow: "auto",
        }}
      >
        {groups.map((g, gi) => (
          <div key={gi}>
            {g.label && (
              <div
                className="sidebar-group-label"
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10,
                  color: "var(--text-dim)",
                  letterSpacing: 0.5,
                  textTransform: "uppercase" as const,
                  padding: "6px 10px 8px",
                }}
              >
                {g.label}
              </div>
            )}
            <div style={{ display: "flex", flexDirection: "column", gap: 1 }}>
              {g.items
                .filter((it) => !it.adminOnly || isAdmin)
                .map((it) => {
                  const on = location.pathname === it.to || (!it.end && location.pathname.startsWith(it.to) && it.to !== "/");
                  return (
                    <NavLink
                      className="sidebar-nav-link"
                      aria-label={it.label}
                      key={it.to}
                      to={it.to}
                      end={it.end}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 10,
                        padding: "7px 10px",
                        borderRadius: "var(--radius-sm)",
                        background: on ? "var(--accent-bg)" : "transparent",
                        color: on ? "var(--accent)" : "var(--text-muted)",
                        fontSize: 13,
                        fontWeight: 500,
                        border: on
                          ? "1px solid var(--accent-border)"
                          : "1px solid transparent",
                        textDecoration: "none",
                        position: "relative" as const,
                      }}
                    >
                      {React.cloneElement(it.icon, { size: 15 })}
                      <span className="sidebar-nav-label" style={{ flex: 1 }}>{it.label}</span>
                      {it.to === "/health" && (
                        <span
                          style={{
                            width: 6,
                            height: 6,
                            borderRadius: 3,
                            background:
                              !healthLoaded
                                ? "var(--text-dim)"
                                : health?.status === "ok" && health?.db_status === "ok"
                                ? "var(--ok)"
                                : health !== null
                                ? "var(--err)"
                                : "var(--text-dim)",
                            opacity: !healthLoaded ? 0.4 : 1,
                          }}
                        />
                      )}
                    </NavLink>
                  );
                })}
            </div>
          </div>
        ))}
      </nav>

      {/* Footer: non-interactive user display */}
      {me && (
        <div className="sidebar-footer" style={{ padding: 14, borderTop: "1px solid var(--border)" }}>
          <div style={{
            display: "flex", alignItems: "center", gap: 10,
            padding: "6px 8px",
          }}>
            <Avatar name={me.display_name} size={28} />
            <div className="sidebar-user-copy" style={{ minWidth: 0, flex: 1 }}>
              <div style={{ fontSize: 13, fontWeight: 500, color: "var(--text)" }}>
                {me.display_name}
              </div>
              <div style={{
                display: "flex", alignItems: "center", gap: 6,
                fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--text-dim)",
              }}>
                <ScopeChip scope={me.scope} />
              </div>
            </div>
          </div>
        </div>
      )}
    </aside>
  );
}
