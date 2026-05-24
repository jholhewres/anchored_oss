import React from "react";
import { useNavigate } from "react-router-dom";
import { Card, Badge, Status, Btn, Input, Tabs, Empty, Avatar } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { Project } from "@/lib/types";

export function ProjectsPage() {
  const navigate = useNavigate();
  const [projects, setProjects] = React.useState<Project[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [filter, setFilter] = React.useState("all");
  const [search, setSearch] = React.useState("");

  React.useEffect(() => {
    api.getProjects()
      .then(setProjects)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const activeProjects = projects.filter(p => !p.deleted_at);
  const filtered = activeProjects.filter(p => {
    if (search && !p.name.toLowerCase().includes(search.toLowerCase()) && !p.slug.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
      <div style={{ display: "flex", gap: 10, marginBottom: 16, alignItems: "center" }}>
        <Input icon={<I.search />} placeholder="Filter by name, tag, team or scope..." size="sm" style={{ width: 360 }} value={search} onChange={e => setSearch(e.target.value)} />
        <Tabs
          active={filter}
          onSet={setFilter}
          tabs={[
            { key: "all", label: "All", count: activeProjects.length },
            { key: "mine", label: "Mine" },
            { key: "shared", label: "Shared" },
            { key: "archived", label: "Archived" },
          ]}
        />
        <div style={{ flex: 1 }} />
        <Badge tone="ok" dot>{activeProjects.length} active</Badge>
      </div>

      {filtered.length === 0 ? (
        <Empty
          icon={<I.folder />}
          title="No projects"
          body="Create your first project to start sharing team memory."
          actions={<Btn variant="primary" size="sm" icon={<I.plus />}>New project</Btn>}
        />
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
          {filtered.map(p => (
            <Card
              key={p.id}
              style={{ padding: 18, cursor: "pointer" }}
              onClick={() => navigate(`/projects/${p.id}`)}
            >
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span style={{ color: "var(--text-muted)", display: "inline-flex" }}><I.folder size={16} /></span>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 14, fontWeight: 500 }}>{p.name}</div>
                </div>
                <Status value="online" label="synced" />
              </div>
              <div style={{ fontSize: 13, color: "var(--text-muted)", minHeight: 36 }}>
                Project · {p.slug}
              </div>
              <div style={{ display: "flex", gap: 5, flexWrap: "wrap", margin: "14px 0" }}>
                <Badge tone="outline">{p.remote_key}</Badge>
              </div>
              <div style={{
                display: "flex", alignItems: "center", justifyContent: "space-between",
                paddingTop: 12, borderTop: "1px solid var(--border)",
                fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)",
              }}>
                <span>{p.created_by ? p.created_by.slice(0, 8) : "unknown"}</span>
                <Avatar name={p.name} size={20} />
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
