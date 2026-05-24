import React from "react";
import { useNavigate } from "react-router-dom";
import { Card, Badge, Btn, Empty, Avatar } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { Team } from "@/lib/types";

export function TeamsPage() {
  const navigate = useNavigate();
  const [teams, setTeams] = React.useState<Team[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    api.getTeams()
      .then(setTeams)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  if (teams.length === 0) {
    return (
      <Empty
        icon={<I.users />}
        title="No teams"
        body="Create your first team to organize members and project access."
        actions={<Btn variant="primary" size="sm" icon={<I.plus />}>New team</Btn>}
      />
    );
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 14 }}>
      {teams.map(t => (
        <Card
          key={t.id}
          style={{ padding: 20, cursor: "pointer" }}
          onClick={() => navigate(`/teams/${t.id}`)}
        >
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 14 }}>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 16, fontWeight: 600 }}>
                team / {t.name}
              </div>
              <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4 }}>
                {t.slug}
              </div>
            </div>
            <Btn variant="ghost" size="sm">View</Btn>
          </div>
          <div style={{
            display: "flex", alignItems: "center", gap: 14,
            padding: "12px 0", borderTop: "1px solid var(--border)", borderBottom: "1px solid var(--border)",
          }}>
            <Avatar name={t.name} size={26} />
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
              created {new Date(t.created_at).toLocaleDateString()}
            </span>
          </div>
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginTop: 12 }}>
            <Badge tone="outline" icon={<I.folder />}>{t.slug}</Badge>
          </div>
        </Card>
      ))}
    </div>
  );
}
