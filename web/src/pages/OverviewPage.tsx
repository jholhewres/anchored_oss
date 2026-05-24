import React from "react";
import { Metric, Card, Btn } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { DashboardStats, AuditEntry } from "@/lib/types";

function ActivityChart() {
  const hours = React.useMemo(() => {
    const seed = [23,45,38,62,78,55,41,89,72,58,95,68,44,82,51,37,66,73,48,85,59,42,77,63];
    return seed.map(t => ({
      writes: Math.round(t * 0.35),
      reads: Math.round(t * 0.65),
    }));
  }, []);
  const max = Math.max(...hours.map(h => h.writes + h.reads));

  return (
    <svg viewBox="0 0 700 220" style={{ width: "100%", height: "100%", overflow: "visible" }}>
      {[0.25, 0.5, 0.75].map(p => (
        <line key={p} x1={0} y1={220 * p} x2={700} y2={220 * p} stroke="var(--border)" strokeDasharray="2 4" />
      ))}
      {hours.map((h, i) => {
        const x = (i / 24) * 700;
        const w = (700 / 24) * 0.7;
        const rH = (h.reads / max) * 200;
        const wH = (h.writes / max) * 200;
        return (
          <g key={i}>
            <rect x={x} y={220 - rH} width={w} height={rH} fill="var(--text-ghost)" rx="1" />
            <rect x={x} y={220 - rH - wH} width={w} height={wH} fill="var(--accent)" rx="1" />
          </g>
        );
      })}
      {[0, 6, 12, 18, 23].map(h => (
        <text key={h} x={(h / 24) * 700} y={220 + 14} fontSize="10" fontFamily="var(--font-mono)" fill="var(--text-dim)">
          {String(h).padStart(2, "0")}:00
        </text>
      ))}
    </svg>
  );
}

const eventIcons: Record<string, React.ReactNode> = {
  "memory.append": <I.cube size={13} />,
  "sync.success": <I.refresh size={13} />,
  "apikey.rotate": <I.key size={13} />,
  "member.invite": <I.users size={13} />,
  "policy.update": <I.shield size={13} />,
  "sync.conflict": <I.alert size={13} />,
  "project.create": <I.folder size={13} />,
  "apikey.create": <I.key size={13} />,
  "team.create": <I.users size={13} />,
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
  "team.create": "neutral",
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
  const [stats, setStats] = React.useState<DashboardStats | null>(null);
  const [audit, setAudit] = React.useState<AuditEntry[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    if (me?.scope !== "admin") { setLoading(false); return; }
    Promise.all([api.getStats(), api.getAudit({ limit: 7 })])
      .then(([s, a]) => { setStats(s); setAudit(a.entries); })
      .catch(() => {})
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
      {isEmpty && (
        <Card style={{ padding: 22, marginBottom: 20, borderStyle: "dashed" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Get started</div>
          <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4, marginBottom: 14 }}>
            Your organization is set up. Create your first project to start sharing memory.
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <Btn variant="primary" size="sm" icon={<I.plus />}>New project</Btn>
            <Btn variant="outline" size="sm" icon={<I.key />}>Generate API key</Btn>
          </div>
        </Card>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14, marginBottom: 28 }}>
        <Metric
          icon={<I.cube />}
          label="memories"
          value={stats ? formatNumber(stats.memories_live) : "—"}
          delta="+128 / 24h"
          trend="up"
          sub={`across ${stats?.projects ?? 0} projects`}
        />
        <Metric
          icon={<I.users />}
          label="active agents"
          value={stats ? String(stats.accounts) : "—"}
          delta={`${stats?.keys_active ?? 0} keys`}
          trend="flat"
          sub={`across ${stats?.teams ?? 0} teams`}
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

      <Card style={{ marginBottom: 14 }}>
        <div style={{ padding: "16px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div>
            <div style={{ fontSize: 15, fontWeight: 500 }}>Activity</div>
            <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 2 }}>Last 24 hours · all projects</div>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 18, fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
              <span style={{ width: 8, height: 8, background: "var(--accent)" }} />writes
            </span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
              <span style={{ width: 8, height: 8, background: "var(--text-ghost)" }} />reads
            </span>
            <span style={{ width: 1, height: 14, background: "var(--border)" }} />
            <Btn variant="ghost" size="sm" iconR={<I.chevD />}>24h</Btn>
          </div>
        </div>
        <div style={{ padding: "22px 22px 10px", height: 240 }}>
          <ActivityChart />
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", borderTop: "1px solid var(--border)", fontFamily: "var(--font-mono)" }}>
          {[
            ["ops total", stats ? formatNumber(stats.audit_entries_24h * 6) : "—"],
            ["projects", stats ? String(stats.projects) : "—"],
            ["active keys", stats ? String(stats.keys_active) : "—"],
            ["teams", stats ? String(stats.teams) : "—"],
          ].map(([k, v], i, a) => (
            <div key={k} style={{ padding: "14px 22px", borderRight: i < a.length - 1 ? "1px solid var(--border)" : "none" }}>
              <div style={{ fontSize: 10.5, color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>{k}</div>
              <div style={{ fontSize: 18, marginTop: 4, color: "var(--text)", fontWeight: 500 }}>{v}</div>
            </div>
          ))}
        </div>
      </Card>

      <Card>
        <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Recent events</div>
          <Btn variant="ghost" size="sm" iconR={<I.arrowR />}>Audit log</Btn>
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
