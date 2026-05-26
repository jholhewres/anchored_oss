import React, { useState, useEffect } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Card, Badge, Status, Btn, Input, Empty } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import {
  type Project,
  type ProjectCategory,
  PROJECT_CATEGORIES,
  PROJECT_CATEGORY_LABELS,
} from "@/lib/types";

const CATEGORY_LABELS_SHORT: Record<ProjectCategory, string> = {
  service: "Service",
  library: "Library",
  app: "App",
  infra: "Infra",
  experiment: "Experiment",
  other: "Other",
};

function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// ── New project modal ───────────────────────────────────────────────────────
interface NewProjectModalProps {
  onClose: () => void;
  onCreated: (p: Project) => void;
}

function NewProjectModal({ onClose, onCreated }: NewProjectModalProps) {
  const toast = useToast();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [category, setCategory] = useState<ProjectCategory>("service");
  const [submitting, setSubmitting] = useState(false);

  function handleNameChange(v: string) {
    setName(v);
    setSlug(slugify(v));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    try {
      const p = await api.createProject({
        name: name.trim(),
        slug: slug.trim() || slugify(name),
        category,
      });
      onCreated(p);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create project";
      toast.push({ title: "Create failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed", inset: 0, zIndex: 50,
        background: "rgba(0,0,0,0.6)",
        display: "flex", alignItems: "center", justifyContent: "center",
      }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          background: "var(--bg-2)", border: "1px solid var(--border)",
          borderRadius: "var(--radius-lg)", padding: 28, width: 420,
          boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
          <div style={{ fontSize: 16, fontWeight: 500 }}>New project</div>
          <button type="button" onClick={onClose} style={{
            border: 0, background: "transparent", color: "var(--text-dim)",
            cursor: "pointer", display: "inline-flex", padding: 4,
          }}><I.x size={16} /></button>
        </div>
        <form onSubmit={submit}>
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
                Name
              </div>
              <Input full size="md" placeholder="my-service" value={name}
                onChange={e => handleNameChange(e.target.value)} required autoFocus />
            </div>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
                Slug
              </div>
              <Input full size="md" placeholder="my-service" value={slug}
                onChange={e => setSlug(e.target.value)} mono />
            </div>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
                Category
              </div>
              <select value={category} onChange={e => setCategory(e.target.value as ProjectCategory)} style={{
                width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
                background: "var(--bg-input)", border: "1px solid var(--border)",
                borderRadius: "var(--radius)", color: "var(--text)", cursor: "pointer",
              }}>
                {PROJECT_CATEGORIES.map(c => (
                  <option key={c} value={c}>{PROJECT_CATEGORY_LABELS[c]}</option>
                ))}
              </select>
            </div>
            <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
              <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
              <Btn variant="primary" size="md" full>
                {submitting ? "Creating…" : "Create project"}
              </Btn>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}

// ── Project card ────────────────────────────────────────────────────────────
function ProjectCard({ p, onClick }: { p: Project; onClick: () => void }) {
  return (
    <Card style={{ padding: 18, cursor: "pointer" }} onClick={onClick}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ color: "var(--text-muted)", display: "inline-flex" }}><I.folder size={16} /></span>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 14, fontWeight: 500 }}>{p.name}</div>
        </div>
        <Status value="online" label="synced" />
      </div>
      <div style={{ fontSize: 13, color: "var(--text-muted)", minHeight: 20, marginBottom: 10 }}>
        {p.slug}
      </div>
      <div style={{ display: "flex", gap: 5, flexWrap: "wrap" as const, marginBottom: 14 }}>
        <Badge tone="outline">{p.remote_key}</Badge>
        <Badge tone="neutral">{CATEGORY_LABELS_SHORT[p.category] ?? p.category}</Badge>
      </div>
      <div style={{
        display: "flex", alignItems: "center", justifyContent: "space-between",
        paddingTop: 10, borderTop: "1px solid var(--border)",
        fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)",
      }}>
        <span>{p.created_by ? p.created_by.slice(0, 8) : "unknown"}</span>
      </div>
    </Card>
  );
}

// ── Main page ───────────────────────────────────────────────────────────────
export function ProjectsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"grouped" | "all">("grouped");
  const [showModal, setShowModal] = useState(false);

  // Open modal automatically when ?new=1
  useEffect(() => {
    if (searchParams.get("new") === "1") {
      setShowModal(true);
      setSearchParams({}, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    api.getProjects()
      .then(setProjects)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const activeProjects = projects.filter(p => !p.deleted_at);
  const filtered = activeProjects.filter(p => {
    if (!search) return true;
    const q = search.toLowerCase();
    return p.name.toLowerCase().includes(q) || p.slug.toLowerCase().includes(q);
  });

  function handleCreated(p: Project) {
    setProjects(prev => [p, ...prev]);
    setShowModal(false);
  }

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
      {/* Toolbar */}
      <div style={{ display: "flex", gap: 10, marginBottom: 16, alignItems: "center" }}>
        <Input icon={<I.search />} placeholder="Filter by name or slug…" size="sm" style={{ width: 320 }}
          value={search} onChange={e => setSearch(e.target.value)} />

        {/* All / Grouped toggle */}
        <div style={{
          display: "flex", gap: 0,
          border: "1px solid var(--border)", borderRadius: "var(--radius)", overflow: "hidden",
        }}>
          {(["grouped", "all"] as const).map(v => (
            <button key={v} type="button" onClick={() => setView(v)} style={{
              padding: "5px 12px", fontSize: 12, fontWeight: 500,
              background: view === v ? "var(--accent-bg)" : "transparent",
              color: view === v ? "var(--accent)" : "var(--text-muted)",
              border: 0, cursor: "pointer", fontFamily: "inherit",
              borderRight: v === "grouped" ? "1px solid var(--border)" : "none",
            }}>
              {v === "grouped" ? "Grouped" : "All"}
            </button>
          ))}
        </div>

        <div style={{ flex: 1 }} />
        <Badge tone="ok" dot>{activeProjects.length} active</Badge>
        <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowModal(true)}>
          New project
        </Btn>
      </div>

      {filtered.length === 0 ? (
        <Empty
          icon={<I.folder />}
          title="No projects"
          body="Create your first project to start sharing team memory."
          actions={<Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowModal(true)}>New project</Btn>}
        />
      ) : view === "all" ? (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
          {filtered.map(p => (
            <ProjectCard key={p.id} p={p} onClick={() => navigate(`/projects/${p.id}`)} />
          ))}
        </div>
      ) : (
        /* Grouped view */
        <div style={{ display: "flex", flexDirection: "column", gap: 28 }}>
          {PROJECT_CATEGORIES.map(cat => {
            const catProjects = filtered.filter(p => (p.category ?? "other") === cat);
            if (catProjects.length === 0) return null;
            return (
              <div key={cat}>
                <div style={{
                  display: "flex", alignItems: "center", gap: 10,
                  marginBottom: 12,
                }}>
                  <span style={{
                    fontFamily: "var(--font-mono)", fontSize: 11, fontWeight: 500,
                    color: "var(--text-muted)", textTransform: "uppercase" as const, letterSpacing: 0.5,
                  }}>
                    {PROJECT_CATEGORY_LABELS[cat]}
                  </span>
                  <Badge tone="neutral">{catProjects.length}</Badge>
                  <span style={{ flex: 1, height: 1, background: "var(--border)" }} />
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
                  {catProjects.map(p => (
                    <ProjectCard key={p.id} p={p} onClick={() => navigate(`/projects/${p.id}`)} />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showModal && (
        <NewProjectModal onClose={() => setShowModal(false)} onCreated={handleCreated} />
      )}
    </div>
  );
}
