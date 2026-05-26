import React from "react";
import { useParams } from "react-router-dom";
import { Card, Badge, Status, Btn, Input, Tabs } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { Project, Memory, Triple } from "@/lib/types";
import { GraphView } from "@/components/GraphView";

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + "..." : s;
}

const categoryTones: Record<string, "accent" | "ok" | "warn" | "err" | "info" | "neutral"> = {
  decision: "accent",
  pattern: "info",
  learning: "warn",
  constraint: "err",
  convention: "neutral",
  fact: "ok",
  plan: "info",
  summary: "neutral",
};

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

function PolicyRow({ title, description, items }: {
  title: string;
  description: string;
  items: { label: string; tone: string; detail: string }[];
}) {
  return (
    <div style={{ borderBottom: "1px solid var(--border)" }}>
      <div style={{ padding: "14px 22px", display: "flex", flexDirection: "column", gap: 4 }}>
        <div style={{ fontSize: 13.5, fontWeight: 500 }}>{title}</div>
        <div style={{ fontSize: 12, color: "var(--text-dim)" }}>{description}</div>
      </div>
      <div style={{ padding: "0 22px 12px" }}>
        {items.map((it, i) => (
          <div key={i} style={{
            display: "flex", alignItems: "flex-start", gap: 10,
            padding: "7px 0",
            borderTop: i > 0 ? "1px solid color-mix(in srgb, var(--border) 50%, transparent)" : "none",
          }}>
            <span style={{
              fontFamily: "var(--font-mono)", fontSize: 10.5, padding: "2px 8px",
              borderRadius: "var(--radius-sm)",
              background: `var(--${it.tone === "neutral" ? "bg-3" : it.tone + "-bg"})`,
              color: `var(--${it.tone === "neutral" ? "text-dim" : it.tone})`,
              border: `1px solid var(--${it.tone === "neutral" ? "border" : it.tone + "-border"})`,
              whiteSpace: "nowrap", flex: "none",
            }}>
              {it.label}
            </span>
            <span style={{ fontSize: 12.5, color: "var(--text-muted)", lineHeight: 1.5 }}>{it.detail}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [project, setProject] = React.useState<Project | null>(null);
  const [memories, setMemories] = React.useState<Memory[]>([]);
  const [memTotal, setMemTotal] = React.useState(0);
  const [offset, setOffset] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [search, setSearch] = React.useState("");
  const [activeTab, setActiveTab] = React.useState("memories");
  const [triples, setTriples] = React.useState<Triple[]>([]);
  const [tripleTotal, setTripleTotal] = React.useState(0);

  React.useEffect(() => {
    if (!id) return;
    Promise.all([
      api.getProject(id),
      api.getProjectMemories(id, 20, 0),
    ])
      .then(([p, m]) => {
        setProject(p);
        setMemories(m.memories);
        setMemTotal(m.total);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [id]);

  React.useEffect(() => {
    if (!id || offset === 0) return;
    api.getProjectMemories(id, 20, offset)
      .then(m => { setMemories(m.memories); setMemTotal(m.total); })
      .catch(() => {});
  }, [id, offset]);

  React.useEffect(() => {
    if (!id || activeTab !== "graph") return;
    api.getProjectGraph(id, 200, 0)
      .then(r => { setTriples(r.triples); setTripleTotal(r.total); })
      .catch(() => {});
  }, [id, activeTab]);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;
  if (!project) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Project not found.</div>;

  const filtered = search
    ? memories.filter(m => m.content.toLowerCase().includes(search.toLowerCase()) || m.category.toLowerCase().includes(search.toLowerCase()))
    : memories;

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
            <I.folder size={20} />
          </div>
          <div>
            <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
              Project · {project.slug}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 16, marginTop: 10, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
              <span><span style={{ color: "var(--text)" }}>{memTotal.toLocaleString()}</span> memories</span>
              <span style={{ color: "var(--text-ghost)" }}>·</span>
              <Badge tone="outline">{project.remote_key}</Badge>
              <span style={{ color: "var(--text-ghost)" }}>·</span>
              <Status value="online" label={`created ${timeAgo(project.created_at)}`} />
            </div>
          </div>
        </div>
      </div>

      <Tabs
        active={activeTab}
        onSet={setActiveTab}
        tabs={[
          { key: "memories", label: "Memories", icon: <I.cube />, count: memTotal },
          { key: "graph", label: "Knowledge graph", icon: <I.graph />, count: tripleTotal },
          { key: "policies", label: "Policies", icon: <I.shield /> },
          { key: "settings", label: "Settings", icon: <I.settings /> },
        ]}
      />

      {activeTab === "memories" && (
        <div style={{ marginTop: 22 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
            <Input icon={<I.search />} placeholder="search memories..." size="sm" style={{ flex: 1, maxWidth: 480 }} value={search} onChange={e => setSearch(e.target.value)} />
            <div style={{ flex: 1 }} />
            <Btn variant="ghost" size="sm" iconR={<I.chevD />}>Newest first</Btn>
          </div>

          {filtered.length === 0 ? (
            <Card style={{ padding: "40px 22px", textAlign: "center" }}>
              <div style={{ fontSize: 13, color: "var(--text-dim)" }}>No memories yet.</div>
            </Card>
          ) : (
            <Card style={{ padding: 0 }}>
              {filtered.map((m, i, arr) => (
                <div key={m.id} style={{
                  padding: "18px 22px", borderBottom: i < arr.length - 1 ? "1px solid var(--border)" : "none",
                  display: "grid", gridTemplateColumns: "110px 1fr 80px 40px", gap: 18, alignItems: "flex-start",
                }}>
                  <Badge tone={categoryTones[m.category] || "neutral"}>{m.category}</Badge>
                  <div>
                    <div style={{ fontSize: 15, fontWeight: 500, marginBottom: 6, letterSpacing: -0.2 }}>
                      {truncate(m.content, 80)}
                    </div>
                    <div style={{ fontSize: 13.5, color: "var(--text-muted)", marginBottom: 10, lineHeight: 1.55, maxWidth: 720 }}>
                      {truncate(m.content, 200)}
                    </div>
                    <div style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)" }}>
                      {m.author_name || "unknown"} · {timeAgo(m.created_at)}
                    </div>
                  </div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)", display: "inline-flex", alignItems: "center", gap: 5 }}>
                    <I.link size={12} />
                  </div>
                  <button style={{ background: "transparent", border: 0, color: "var(--text-dim)", padding: 6, cursor: "pointer", borderRadius: 4 }}>
                    <I.more size={14} />
                  </button>
                </div>
              ))}
            </Card>
          )}

          <div style={{ marginTop: 14, display: "flex", justifyContent: "space-between", alignItems: "center", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
            <span>showing {filtered.length} of {memTotal.toLocaleString()} · sorted by recency</span>
            <div style={{ display: "flex", gap: 8 }}>
              <Btn variant="ghost" size="sm" style={offset === 0 ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setOffset(Math.max(0, offset - 20))}>Previous</Btn>
              <Btn variant="ghost" size="sm" style={offset + 20 >= memTotal ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setOffset(offset + 20)}>Load more</Btn>
            </div>
          </div>
        </div>
      )}

      {activeTab === "graph" && (
        <div style={{ marginTop: 22 }}>
          <GraphView triples={triples} total={tripleTotal} />
        </div>
      )}

      {activeTab === "policies" && (
        <div style={{ marginTop: 22 }}>
          <Card style={{ padding: 0 }}>
            <div style={{ padding: "16px 22px", borderBottom: "1px solid var(--border)" }}>
              <div style={{ fontSize: 15, fontWeight: 500 }}>Sync guardrails</div>
              <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 4 }}>
                Server-side rules enforced on every incoming sync. Non-negotiable.
              </div>
            </div>

            <div style={{ padding: 0 }}>
              <PolicyRow
                title="Blocked categories"
                description="These categories are local-only and rejected at sync time."
                items={[
                  { label: "event", tone: "err", detail: "Session events are transient and machine-local" },
                  { label: "preference", tone: "err", detail: "User preferences are personal, not team-shared" },
                ]}
              />

              <PolicyRow
                title="Allowed categories"
                description="Only these categories are accepted into the project."
                items={[
                  { label: "fact", tone: "ok", detail: "Stable project truths" },
                  { label: "decision", tone: "accent", detail: "Architectural and product decisions" },
                  { label: "plan", tone: "info", detail: "Forward-looking intent" },
                  { label: "summary", tone: "neutral", detail: "Consolidated recaps" },
                  { label: "learning", tone: "warn", detail: "Non-obvious lessons and insights" },
                ]}
              />

              <PolicyRow
                title="Content filtering"
                description="Patterns detected in memory content that trigger rejection."
                items={[
                  { label: "Local paths", tone: "err", detail: "/home/..., /Users/..., ~/..., C:\\Users\\..., /tmp/... — use repo-relative paths" },
                  { label: "Secrets", tone: "err", detail: "Stripe/GitHub/Slack tokens, AWS keys, PEM private keys, credential URIs" },
                  { label: "User-scoped", tone: "warn", detail: "Memories with metadata.scope = 'user' are personal, not team" },
                  { label: "Operational", tone: "warn", detail: "metadata.memory_type = 'operational' is local session context" },
                  { label: "Pre-compact/handoff", tone: "warn", detail: "metadata.origin = 'precompact'|'handoff' — local session artifacts" },
                ]}
              />

              <PolicyRow
                title="Storage quota"
                description="Per-organization limits."
                items={[
                  { label: "1 GB (cloud)", tone: "info", detail: "Free tier storage limit per org in cloud mode" },
                  { label: "Unlimited (self-hosted)", tone: "ok", detail: "No storage limit when running your own server" },
                ]}
              />
            </div>
          </Card>
        </div>
      )}

      {activeTab === "settings" && (
        <div style={{ marginTop: 22 }}>
          <Card style={{ padding: 22 }}>
            <div style={{ display: "grid", gap: 14 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--border)" }}>
                <span style={{ color: "var(--text-dim)" }}>ID</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{project.id}</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--border)" }}>
                <span style={{ color: "var(--text-dim)" }}>Slug</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{project.slug}</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--border)" }}>
                <span style={{ color: "var(--text-dim)" }}>Remote key</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{project.remote_key}</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0", borderBottom: "1px solid var(--border)" }}>
                <span style={{ color: "var(--text-dim)" }}>Created by</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{project.created_by}</span>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 0" }}>
                <span style={{ color: "var(--text-dim)" }}>Created</span>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{new Date(project.created_at).toLocaleString()}</span>
              </div>
            </div>
          </Card>
        </div>
      )}
    </div>
  );
}
