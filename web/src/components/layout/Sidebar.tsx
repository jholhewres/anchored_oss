import React from "react";
import { NavLink, useLocation } from "react-router-dom";

import { I, AnchoredLogo } from "@/ds/icons";
import { Avatar, ScopeChip } from "@/ds/components";
import { useAuth } from "@/lib/auth";

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
      { to: "/", icon: <I.home />, label: "Overview", end: true },
      { to: "/projects", icon: <I.folder />, label: "Projects" },
      { to: "/accounts", icon: <I.user />, label: "Accounts", adminOnly: true },
      { to: "/teams", icon: <I.users />, label: "Teams" },
    ],
  },
  {
    label: "Admin",
    items: [
      { to: "/api-keys", icon: <I.key />, label: "API keys", adminOnly: true },
      { to: "/audit", icon: <I.activity />, label: "Audit log", adminOnly: true },
      { to: "/health", icon: <I.pulse />, label: "Health", status: "ok" },
    ],
  },
];

export function Sidebar() {
  const { me, logout } = useAuth();
  const location = useLocation();
  const isAdmin = me?.scope === "admin";

  return (
    <aside
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
      <div style={{ padding: "16px 14px", borderBottom: "1px solid var(--border)" }}>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "8px 10px",
            borderRadius: "var(--radius)",
            background: "var(--bg-2)",
            border: "1px solid var(--border)",
          }}
        >
          <AnchoredLogo size={18} wordmark={false} />
          <div style={{ minWidth: 0, flex: 1 }}>
            <div style={{ fontSize: 13, fontWeight: 500, letterSpacing: -0.2 }}>
              anchored
            </div>
            <div
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 10.5,
                color: "var(--text-dim)",
              }}
            >
              org · self-hosted
            </div>
          </div>
          <I.chevD size={14} />
        </div>
      </div>

      <nav
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
                      <span style={{ flex: 1 }}>{it.label}</span>
                      {it.status && (
                        <span
                          style={{
                            width: 6,
                            height: 6,
                            borderRadius: 3,
                            background: "var(--ok)",
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

      {me && (
        <div style={{ padding: 14, borderTop: "1px solid var(--border)" }}>
          <button
            onClick={() => {
              logout();
              window.location.replace("/login");
            }}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              padding: "6px 8px",
              borderRadius: "var(--radius)",
              background: "transparent",
              border: 0,
              cursor: "pointer",
              width: "100%",
              textAlign: "left",
              color: "inherit",
              fontFamily: "inherit",
            }}
          >
            <Avatar name={me.display_name} size={28} />
            <div style={{ minWidth: 0, flex: 1 }}>
              <div style={{ fontSize: 13, fontWeight: 500, color: "var(--text)" }}>
                {me.display_name}
              </div>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontFamily: "var(--font-mono)",
                  fontSize: 10.5,
                  color: "var(--text-dim)",
                }}
              >
                <ScopeChip scope={me.scope} />
              </div>
            </div>
            <span style={{ color: "var(--text-muted)" }}><I.settings size={14} /></span>
          </button>
        </div>
      )}
    </aside>
  );
}
