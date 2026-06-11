import { useEffect, useState } from "react";
import { api, TaskThread } from "@/lib/api";
import { Card, Badge, Empty } from "@/ds/components";

// MyTasksPage is the personal kanban: a READ-MOSTLY view of the caller's own
// task threads, synced from their anchored client. Cards are auto-populated
// (branch inference + `anchored task`) — this page deliberately has no CRUD:
// the source of truth for task management stays in Jira/Trello; what anchored
// adds is WHERE in the code the task lived and what was learned along the way.
const columns: { status: string; label: string; tone: "ok" | "warn" | "neutral" }[] = [
  { status: "active", label: "Active", tone: "ok" },
  { status: "paused", label: "Paused", tone: "warn" },
  { status: "done", label: "Done", tone: "neutral" },
];

export function MyTasksPage() {
  const [threads, setThreads] = useState<TaskThread[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .listMyTaskThreads()
      .then((r) => setThreads(r.threads ?? []))
      .catch((e) => setError(e?.message ?? "failed to load task threads"))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div style={{ padding: 24 }}>Loading…</div>;
  if (error) return <div style={{ padding: 24, color: "var(--err)" }}>{error}</div>;

  const byStatus = (s: string) =>
    threads.filter((t) => (s === "done" ? t.status === "done" || t.status === "cancelled" : t.status === s));

  if (threads.length === 0) {
    return (
      <div style={{ padding: 24 }}>
        <Empty
          title="No task threads yet"
          body="Work on a branch named after a ticket (feature/PROJ-123-…) or run `anchored task start PROJ-123` — threads sync here automatically."
        />
      </div>
    );
  }

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, alignItems: "start" }}>
        {columns.map((col) => (
          <div key={col.status}>
            <div style={{ marginBottom: 8, display: "flex", alignItems: "center", gap: 8 }}>
              <strong>{col.label}</strong>
              <Badge tone={col.tone}>{byStatus(col.status).length}</Badge>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              {byStatus(col.status).map((t) => (
                <Card key={t.task_key}>
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 8 }}>
                    <strong>{t.task_key}</strong>
                    {t.status === "cancelled" && <Badge tone="err">cancelled</Badge>}
                  </div>
                  {t.external_ref && /^https?:\/\//.test(t.external_ref) && (
                    <div style={{ marginTop: 4 }}>
                      <a href={t.external_ref} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>
                        {t.external_ref}
                      </a>
                    </div>
                  )}
                  {t.projects && t.projects.length > 0 && (
                    <div style={{ marginTop: 8, display: "flex", flexWrap: "wrap", gap: 6 }}>
                      {t.projects.map((p) => (
                        <Badge key={p} tone="neutral">
                          {p}
                        </Badge>
                      ))}
                    </div>
                  )}
                  {/* SAFETY: journal is user-supplied text — render as text only,
                      never via dangerouslySetInnerHTML or markdown. */}
                  {t.journal && t.journal.length > 0 && (
                    <ul style={{ marginTop: 8, paddingLeft: 16, fontSize: 12, color: "var(--text-dim)" }}>
                      {t.journal.slice(0, 3).map((n, i) => (
                        <li key={i}>{n}</li>
                      ))}
                    </ul>
                  )}
                  <div style={{ marginTop: 8, fontSize: 11, color: "var(--text-dim)" }}>
                    updated {new Date(t.updated_at).toLocaleString()}
                  </div>
                </Card>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
