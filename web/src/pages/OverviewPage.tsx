import React from "react";
import { useNavigate, Link } from "react-router-dom";
import { Metric, Card, Btn } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { DashboardStats, AuditEntry } from "@/lib/types";

const eventIcons: Record<string, React.ReactNode> = {
  "memory.append": <I.cube size={13} />,
  "sync.success": <I.refresh size={13} />,
  "apikey.rotate": <I.key size={13} />,
  "member.invite": <I.users size={13} />,
  "policy.update": <I.shield size={13} />,
  "sync.conflict": <I.alert size={13} />,
  "project.create": <I.folder size={13} />,
  "apikey.create": <I.key size={13} />,
};

const actionTones: Record<string, string> = {
  "memory.append": "accent",
  "sync.success": "ok",
  "apikey.rotate": "warn",
  "member.invite": "info",
  "policy.update": "neutral",
  "sync.conflict": "err",
  "project.create": "info",
  "apikey.create": "ok",
};

function formatNumber(n: number): string {
  return n.toLocaleString();
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

export function OverviewPage() {
  const { me } = useAuth();
  const navigate = useNavigate();
  const [stats, setStats] = React.useState<DashboardStats | null>(null);
  const [audit, setAudit] = React.useState<AuditEntry[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [firstLogin] = React.useState(() => {
    const v = localStorage.getItem("anchored_first_login");
    if (v === "1") { localStorage.removeItem("anchored_first_login"); return true; }
    return false;
  });

  React.useEffect(() => {
    if (me?.scope !== "admin") { setLoading(false); return; }
    Promise.all([api.getStats(), api.getAudit({ limit: 7 })])
      .then(([s, a]) => { setStats(s); setAudit(a.entries); })
      .catch((err) => { setError(err instanceof Error ? err.message : "Failed to load dashboard data"); })
      .finally(() => setLoading(false));
  }, [me]);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  if (me?.scope !== "admin") {
    return (
      <div>
        <h1 style={{ fontSize: 22, fontWeight: 500, letterSpacing: -0.5, margin: 0, lineHeight: 1.1 }}>Overview</h1>
        <Card style={{ marginTop: 20, padding: 22 }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Admin scope required</div>
          <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4 }}>
            The overview dashboard aggregates org-level data and is restricted to admin keys.
          </div>
        </Card>
      </div>
    );
  }

  const isEmpty = stats && stats.projects === 0;

  return (
    <div>
      {/* Fetch error banner */}
      {error && (
        <Card style={{ padding: 16, marginBottom: 20, background: "var(--err-bg)", border: "1px solid color-mix(in srgb, var(--err) 25%, transparent)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, color: "var(--err)" }}>
            <I.alert size={14} />
            <span style={{ fontSize: 13 }}>Couldn't load overview data — {error}</span>
          </div>
        </Card>
      )}

      {/* First-login welcome banner */}
      {firstLogin && (
        <Card style={{ padding: 18, marginBottom: 20, border: "1px solid var(--accent-border)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
            <div style={{
              width: 32, height: 32, borderRadius: 6, background: "var(--accent-bg)", color: "var(--accent)",
              display: "inline-flex", alignItems: "center", justifyContent: "center",
              border: "1px solid var(--accent-border)", flex: "none",
            }}>
              <I.key size={16} />
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 500 }}>Welcome — create your first API key</div>
              <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 2 }}>
                Generate a key so your agents can start syncing memory.
              </div>
            </div>
            <Btn variant="primary" size="sm" icon={<I.key />} onClick={() => navigate("/api-keys")}>
              Generate API key
            </Btn>
          </div>
        </Card>
      )}

      {/* Get started card when no projects */}
      {isEmpty && (
        <Card style={{ padding: 22, marginBottom: 20, borderStyle: "dashed" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Get started</div>
          <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4, marginBottom: 14 }}>
            Your organization is set up. Create your first project to start sharing memory.
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => navigate("/projects?new=1")}>
              New project
            </Btn>
            <Btn variant="outline" size="sm" icon={<I.key />} onClick={() => navigate("/api-keys")}>
              Generate API key
            </Btn>
          </div>
        </Card>
      )}

      {/* Shortcut row */}
      <div style={{ display: "flex", gap: 8, marginBottom: 24 }}>
        <Btn variant="outline" size="sm" icon={<I.plus />} onClick={() => navigate("/projects?new=1")}>
          New project
        </Btn>
        <Btn variant="outline" size="sm" icon={<I.users />} onClick={() => navigate("/developers")}>
          Invite developer
        </Btn>
        <Btn variant="outline" size="sm" icon={<I.key />} onClick={() => navigate("/api-keys")}>
          Generate API key
        </Btn>
        <Btn variant="ghost" size="sm" icon={<I.external />} as="a" href="https://github.com/jholhewres/anchored_oss#readme" target="_blank" rel="noopener noreferrer">
          Docs
        </Btn>
      </div>

      {/* Metrics */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14, marginBottom: 28 }}>
        <Metric
          icon={<I.cube />}
          label="memories"
          value={stats ? formatNumber(stats.memories_live) : "—"}
          trend="flat"
          sub={`across ${stats?.projects ?? 0} projects`}
        />
        <Metric
          icon={<I.users />}
          label="active agents"
          value={stats ? String(stats.accounts) : "—"}
          delta={`${stats?.keys_active ?? 0} keys`}
          trend="flat"
          sub="team members"
        />
        <Metric
          icon={<I.clock />}
          label="audit 24h"
          value={stats ? formatNumber(stats.audit_entries_24h) : "—"}
          delta="events"
          trend="flat"
          sub="last 24 hours"
        />
      </div>

      {/* Recent pushes */}
      <Card style={{ marginBottom: 14 }}>
        <div style={{ padding: "16px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div>
            <div style={{ fontSize: 15, fontWeight: 500 }}>Recent pushes</div>
            <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 2 }}>Memory sync activity · all projects</div>
          </div>
        </div>
        <div>
          {(!stats?.recent_pushes || stats.recent_pushes.length === 0) ? (
            <div style={{ padding: "40px 22px", textAlign: "center" }}>
              <div style={{ color: "var(--text-dim)", fontSize: 13, marginBottom: 8 }}>No pushes yet</div>
              <div style={{ fontSize: 12, color: "var(--text-ghost)" }}>
                Run <code style={{ fontFamily: "var(--font-mono)" }}>anchored sync</code> from a project to see activity here.
              </div>
            </div>
          ) : (
            stats.recent_pushes.map((p, i, arr) => (
              <div key={p.project_id} style={{
                display: "grid", gridTemplateColumns: "1fr 100px 140px",
                alignItems: "center", gap: 14, padding: "12px 22px",
                borderBottom: i < arr.length - 1 ? "1px solid var(--border)" : "none",
              }}>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span style={{ color: "var(--text-muted)", display: "inline-flex" }}><I.folder size={14} /></span>
                  <span style={{ fontFamily: "var(--font-mono)", fontSize: 13 }}>{p.project_name}</span>
                </div>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-muted)" }}>
                  {p.count} push{p.count !== 1 ? "es" : ""}
                </span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", textAlign: "right" }}>
                  {timeAgo(p.last_push)}
                </span>
              </div>
            ))
          )}
        </div>
        {/* Quick stats footer */}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", borderTop: "1px solid var(--border)", fontFamily: "var(--font-mono)" }}>
          {[
            ["projects", stats ? String(stats.projects) : "—"],
            ["active keys", stats ? String(stats.keys_active) : "—"],
            ["members", stats ? String(stats.accounts) : "—"],
          ].map(([k, v], i, a) => (
            <div key={k} style={{ padding: "14px 22px", borderRight: i < a.length - 1 ? "1px solid var(--border)" : "none" }}>
              <div style={{ fontSize: 10.5, color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>{k}</div>
              <div style={{ fontSize: 18, marginTop: 4, color: "var(--text)", fontWeight: 500 }}>{v}</div>
            </div>
          ))}
        </div>
      </Card>

      {/* Recent events */}
      <Card>
        <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Recent events</div>
          <Link to="/audit" style={{ textDecoration: "none" }}>
            <Btn variant="ghost" size="sm" iconR={<I.arrowR />}>Audit log</Btn>
          </Link>
        </div>
        <div>
          {audit.length === 0 && (
            <div style={{ padding: "32px 22px", textAlign: "center", color: "var(--text-dim)", fontSize: 13 }}>No recent events</div>
          )}
          {audit.map((e, i, arr) => {
            const tone = actionTones[e.action] || "neutral";
            const icon = eventIcons[e.action] || <I.activity size={13} />;
            const toneVar = tone === "neutral" ? "text-muted" : tone === "accent" ? "accent" : tone;
            return (
              <div key={e.id} style={{
                display: "grid", gridTemplateColumns: "28px 200px 140px 1fr 90px",
                alignItems: "center", gap: 14, padding: "12px 22px",
                borderBottom: i < arr.length - 1 ? "1px solid var(--border)" : "none",
              }}>
                <span style={{
                  width: 24, height: 24, borderRadius: 5,
                  background: `var(--${tone === "neutral" ? "bg-3" : tone + "-bg"})`,
                  color: `var(--${toneVar})`,
                  display: "inline-flex", alignItems: "center", justifyContent: "center",
                }}>
                  {icon}
                </span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12.5, color: "var(--text)" }}>{e.action}</span>
                <span style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
                  {e.actor_id ? e.actor_id.slice(0, 8) : "system"}
                </span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
                  {e.target_type ? `${e.target_type}/${e.target_id?.slice(0, 8)}` : ""}
                </span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", textAlign: "right" }}>
                  {timeAgo(e.created_at)}
                </span>
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
}
