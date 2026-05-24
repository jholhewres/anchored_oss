import React from "react";
import { useParams } from "react-router-dom";
import { Card, Avatar, Table, ScopeChip } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { TeamDetail } from "@/lib/types";

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

export function TeamDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [team, setTeam] = React.useState<TeamDetail | null>(null);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    if (!id) return;
    api.getTeam(id)
      .then(setTeam)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;
  if (!team) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Team not found.</div>;

  return (
    <div>
      <div style={{
        display: "flex", alignItems: "flex-start", justifyContent: "space-between",
        gap: 24, marginBottom: 24, paddingBottom: 22, borderBottom: "1px solid var(--border)",
      }}>
        <div style={{ display: "flex", alignItems: "flex-start", gap: 16 }}>
          <div style={{
            width: 44, height: 44, borderRadius: "var(--radius)",
            background: "var(--accent-bg)", color: "var(--accent)",
            border: "1px solid var(--accent-border)",
            display: "inline-flex", alignItems: "center", justifyContent: "center", flex: "none",
          }}>
            <I.users size={20} />
          </div>
          <div>
            <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
              team / {team.slug}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 16, marginTop: 10, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
              <span><span style={{ color: "var(--text)" }}>{team.members.length}</span> members</span>
              <span style={{ color: "var(--text-ghost)" }}>·</span>
              <span><span style={{ color: "var(--text)" }}>{team.project_grants.length}</span> projects</span>
            </div>
          </div>
        </div>
      </div>

      <Card style={{ marginBottom: 20 }}>
        <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Members</div>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>{team.members.length} total</span>
        </div>
        {team.members.length === 0 ? (
          <div style={{ padding: "32px 22px", textAlign: "center", color: "var(--text-dim)", fontSize: 13 }}>No members yet.</div>
        ) : (
          <Table
            cols={[
              { key: "name", label: "Account" },
              { key: "email", label: "Email", mono: true, muted: true },
              { key: "added", label: "Added", mono: true, muted: true },
            ]}
            rows={team.members.map(m => ({
              name: (
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <Avatar name={m.display_name} size={28} />
                  <span style={{ fontWeight: 500 }}>{m.display_name}</span>
                </div>
              ),
              email: m.email,
              added: timeAgo(m.added_at),
            }))}
          />
        )}
      </Card>

      <Card>
        <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ fontSize: 15, fontWeight: 500 }}>Project access</div>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>{team.project_grants.length} grants</span>
        </div>
        {team.project_grants.length === 0 ? (
          <div style={{ padding: "32px 22px", textAlign: "center", color: "var(--text-dim)", fontSize: 13 }}>No project grants yet.</div>
        ) : (
          <Table
            cols={[
              { key: "project", label: "Project" },
              { key: "slug", label: "Slug", mono: true, muted: true },
              { key: "role", label: "Role" },
            ]}
            rows={team.project_grants.map(g => ({
              project: <span style={{ fontWeight: 500 }}>{g.project_name}</span>,
              slug: g.project_slug,
              role: <ScopeChip scope={g.role} />,
            }))}
          />
        )}
      </Card>
    </div>
  );
}
