import { useEffect, useMemo, useState } from "react";

import { Badge, Btn, Card, Empty, Input } from "@/ds/components";
import { I } from "@/ds/icons";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { SkillStatus } from "@/lib/types";
import { CatalogSkill, SkillEditor, type SkillTemplate } from "@/components/SkillEditor";

function formatError(error: unknown, fallback: string) {
  return error instanceof ApiError || error instanceof Error ? error.message : fallback;
}

function statusTone(status: SkillStatus): "ok" | "warn" | "err" | "neutral" {
  if (status === "active") return "ok";
  if (status === "draft") return "warn";
  if (status === "revoked") return "err";
  return "neutral";
}

type EditorState = { skill?: CatalogSkill; template?: SkillTemplate } | null;

function TemplatePicker({ templates, loading, error, onChoose, onClose }: {
  templates: SkillTemplate[];
  loading: boolean;
  error: string | null;
  onChoose: (template: SkillTemplate | null) => void;
  onClose: () => void;
}) {
  return (
    <div role="presentation" onMouseDown={onClose} style={{ position: "fixed", inset: 0, zIndex: 60, display: "flex", alignItems: "center", justifyContent: "center", padding: 20, background: "color-mix(in srgb, black 55%, transparent)" }}>
      <div role="dialog" aria-modal="true" aria-labelledby="skill-source-title" onMouseDown={(event) => event.stopPropagation()} style={{ width: "min(720px, 100%)", maxHeight: "min(720px, 100%)", overflow: "auto", padding: 24, background: "var(--bg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius-lg)", boxShadow: "0 24px 64px rgba(0,0,0,.35)" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 8 }}>
          <div>
            <h2 id="skill-source-title" style={{ margin: 0, fontSize: 17 }}>Start an organization skill</h2>
            <p style={{ margin: "5px 0 0", color: "var(--text-muted)", fontSize: 12.5, lineHeight: 1.45 }}>Use a blank skill or copy a versioned built-in template. Templates are never linked after creation.</p>
          </div>
          <Btn variant="ghost" size="sm" icon={<I.x />} aria-label="Close template picker" onClick={onClose} />
        </div>
        {error && <div role="alert" style={{ margin: "14px 0", padding: "10px 12px", border: "1px solid color-mix(in srgb, var(--err) 35%, transparent)", background: "var(--err-bg)", color: "var(--err)", borderRadius: "var(--radius)", fontSize: 12.5 }}>{error}</div>}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))", gap: 12, marginTop: 18 }}>
          <button type="button" onClick={() => onChoose(null)} style={{ padding: 16, textAlign: "left", background: "var(--bg-2)", border: "1px solid var(--border)", borderRadius: "var(--radius)", color: "inherit", cursor: "pointer", fontFamily: "inherit" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 7 }}><I.plus size={15} /><strong style={{ fontSize: 13.5 }}>Blank skill</strong></div>
            <span style={{ color: "var(--text-muted)", fontSize: 12.5, lineHeight: 1.45 }}>Write a new instruction set from scratch.</span>
          </button>
          {loading && <div style={{ padding: 16, color: "var(--text-dim)", fontSize: 13 }}>Loading built-in templates…</div>}
          {templates.map((template) => (
            <button key={`${template.id}@${template.revision}`} type="button" onClick={() => onChoose(template)} style={{ padding: 16, textAlign: "left", background: "var(--bg-2)", border: "1px solid var(--border)", borderRadius: "var(--radius)", color: "inherit", cursor: "pointer", fontFamily: "inherit" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, marginBottom: 7 }}><strong style={{ fontSize: 13.5 }}>{template.name}</strong><Badge tone="neutral">{template.revision}</Badge></div>
              <div style={{ color: "var(--text-muted)", fontSize: 12.5, lineHeight: 1.45 }}>{template.purpose}</div>
              <code style={{ display: "block", marginTop: 8, color: "var(--text-dim)", fontSize: 11 }}>{template.id}</code>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function SkillCard({ skill, canManage, onEdit }: { skill: CatalogSkill; canManage: boolean; onEdit: (skill: CatalogSkill) => void }) {
  const projectCount = skill.project_count ?? 0;
  return (
    <Card style={{ padding: 18 }}>
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}><strong style={{ fontSize: 14 }}>{skill.name}</strong><Badge tone={statusTone(skill.status)}>{skill.status}</Badge></div>
          <p style={{ margin: "8px 0", minHeight: 38, color: "var(--text-muted)", fontSize: 12.5, lineHeight: 1.5 }}>{skill.purpose}</p>
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap", color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 11.5 }}>
            <code>{skill.slug}@{skill.version}</code><span>{projectCount} {projectCount === 1 ? "project" : "projects"}</span>
          </div>
        </div>
        {canManage && <Btn variant="outline" size="sm" onClick={() => onEdit(skill)}>Edit</Btn>}
      </div>
    </Card>
  );
}

export function SkillsPage() {
  const { me } = useAuth();
  const canManage = me?.scope === "admin";
  const [skills, setSkills] = useState<CatalogSkill[]>([]);
  const [templates, setTemplates] = useState<SkillTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [showPicker, setShowPicker] = useState(false);
  const [editor, setEditor] = useState<EditorState>(null);

  const loadSkills = async () => {
    setLoading(true);
    setError(null);
    try {
      setSkills(await api.getSkills());
    } catch (reason) {
      setError(formatError(reason, "Could not load the organization skill catalog."));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void loadSkills(); }, []);

  const openPicker = async () => {
    setShowPicker(true);
    if (templates.length > 0) return;
    setTemplatesLoading(true);
    setTemplateError(null);
    try {
      setTemplates(await api.getSkillTemplates());
    } catch (reason) {
      setTemplateError(formatError(reason, "Could not load built-in templates. You can still start with a blank skill."));
    } finally {
      setTemplatesLoading(false);
    }
  };

  const filteredSkills = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    if (!normalizedQuery) return skills;
    return skills.filter((skill) => `${skill.name} ${skill.slug} ${skill.purpose}`.toLowerCase().includes(normalizedQuery));
  }, [query, skills]);

  const saveSkill = (saved: CatalogSkill) => {
    setSkills((current) => {
      const index = current.findIndex((skill) => skill.id === saved.id);
      return index < 0 ? [saved, ...current] : current.map((skill) => skill.id === saved.id ? saved : skill);
    });
  };

  return (
    <div>
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16, flexWrap: "wrap", marginBottom: 20 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 21, letterSpacing: -0.4 }}>Organization skills</h1>
          <p style={{ margin: "6px 0 0", maxWidth: 680, color: "var(--text-muted)", fontSize: 13, lineHeight: 1.5 }}>Versioned, shared instructions. Attach each skill to the projects that should receive it.</p>
        </div>
        {canManage && <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => void openPicker()}>New skill</Btn>}
      </div>
      {!canManage && <div style={{ marginBottom: 16, padding: "10px 12px", border: "1px solid var(--border)", borderRadius: "var(--radius)", color: "var(--text-dim)", fontSize: 12.5 }}>You can browse this catalog; organization administrators manage publishing and project attachments.</div>}
      {error && <div role="alert" style={{ marginBottom: 16, padding: "10px 12px", border: "1px solid color-mix(in srgb, var(--err) 35%, transparent)", background: "var(--err-bg)", color: "var(--err)", borderRadius: "var(--radius)", fontSize: 12.5 }}>{error}</div>}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 16 }}>
        <Input full size="sm" icon={<I.search />} placeholder="Search by name, slug, or purpose…" value={query} onChange={(event) => setQuery(event.target.value)} style={{ maxWidth: 460 }} />
        <span style={{ color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 11.5 }}>{filteredSkills.length} skills</span>
      </div>
      {loading ? <div style={{ padding: 40, color: "var(--text-dim)" }}>Loading skills…</div>
        : filteredSkills.length === 0 ? <Empty icon={<I.layers />} title={skills.length === 0 ? "No organization skills" : "No matching skills"} body={skills.length === 0 ? "Create a reusable instruction set, then attach it to one or more projects." : "Try another catalog search."} actions={canManage && skills.length === 0 ? <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => void openPicker()}>New skill</Btn> : undefined} />
          : <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(285px, 1fr))", gap: 14 }}>{filteredSkills.map((skill) => <SkillCard key={skill.id} skill={skill} canManage={canManage} onEdit={(selected) => setEditor({ skill: selected })} />)}</div>}
      {showPicker && <TemplatePicker templates={templates} loading={templatesLoading} error={templateError} onClose={() => setShowPicker(false)} onChoose={(template) => { setShowPicker(false); setEditor(template ? { template } : {}); }} />}
      {editor && <SkillEditor skill={editor.skill} template={editor.template} onClose={() => setEditor(null)} onSaved={saveSkill} />}
    </div>
  );
}
