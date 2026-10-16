import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { Badge, Btn, Card, Input } from "@/ds/components";
import { I } from "@/ds/icons";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Skill, SkillStatus } from "@/lib/types";

function formatError(error: unknown, fallback: string) {
  return error instanceof ApiError || error instanceof Error ? error.message : fallback;
}

function badgeTone(status: SkillStatus): "ok" | "warn" | "err" | "neutral" {
  if (status === "active") return "ok";
  if (status === "draft") return "warn";
  if (status === "revoked") return "err";
  return "neutral";
}

function SkillRow({ skill, attached, busy, canManage, onAttach }: {
  skill: Skill;
  attached: boolean;
  busy: boolean;
  canManage: boolean;
  onAttach: (skill: Skill, shouldAttach: boolean) => void;
}) {
  return (
    <div style={{ padding: "14px 16px", borderTop: "1px solid var(--border)", display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: 14, alignItems: "center" }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 5 }}>
          <strong style={{ fontSize: 13.5 }}>{skill.name}</strong>
          <Badge tone={badgeTone(skill.status)}>{skill.status}</Badge>
          {attached && <Badge tone="accent">attached</Badge>}
        </div>
        <p style={{ margin: "0 0 7px", fontSize: 12.5, color: "var(--text-muted)", lineHeight: 1.45 }}>{skill.purpose}</p>
        <code style={{ color: "var(--text-dim)", fontSize: 11.5 }}>{skill.slug}@{skill.version}</code>
      </div>
      {canManage && (
        <Btn
          variant={attached ? "outline" : "primary"}
          size="sm"
          disabled={busy || (skill.status !== "active" && !attached)}
          onClick={() => onAttach(skill, !attached)}
        >
          {attached ? "Detach" : "Attach"}
        </Btn>
      )}
    </div>
  );
}

/** Select existing organization skills for this project; authoring lives in /skills. */
export function ProjectSkillsTab({ projectId }: { projectId: string }) {
  const { me } = useAuth();
  const canManage = me?.scope === "admin";
  const [catalog, setCatalog] = useState<Skill[]>([]);
  const [attached, setAttached] = useState<Skill[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const reload = async () => {
    setLoading(true);
    setError(null);
    try {
      const projectSkills = canManage
        ? await api.getProjectSkillAttachments(projectId)
        : await api.getProjectSkills(projectId);
      setAttached(projectSkills);
      setCatalog(canManage ? await api.getSkills() : projectSkills);
    } catch (reason) {
      setError(formatError(reason, "Could not load project skills."));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void reload(); }, [projectId, canManage]);

  const attachedIds = useMemo(() => new Set(attached.map((skill) => skill.id)), [attached]);
  const matchingSkills = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    if (!normalizedQuery) return catalog;
    return catalog.filter((skill) => `${skill.name} ${skill.slug} ${skill.purpose}`.toLowerCase().includes(normalizedQuery));
  }, [catalog, query]);

  const setAttachment = async (skill: Skill, shouldAttach: boolean) => {
    const nextIds = shouldAttach ? [...attachedIds, skill.id] : [...attachedIds].filter((id) => id !== skill.id);
    setBusy(skill.id);
    setError(null);
    try {
      await api.setProjectSkills(projectId, nextIds);
      await reload();
      setNotice(`${shouldAttach ? "Attached" : "Detached"} ${skill.name}.`);
    } catch (reason) {
      setError(formatError(reason, `Could not ${shouldAttach ? "attach" : "detach"} skill.`));
    } finally {
      setBusy(null);
    }
  };

  return (
    <section id="project-tabs-panel-skills" role="tabpanel" aria-labelledby="project-tabs-tab-skills" style={{ marginTop: 22 }}>
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16, flexWrap: "wrap", marginBottom: 18 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 16 }}>Project skills</h2>
          <p style={{ margin: "5px 0 0", color: "var(--text-muted)", fontSize: 13, lineHeight: 1.5 }}>Attach reviewed organization skills to this project. Changes are shared; no project-local copies are created.</p>
        </div>
        {canManage && <Link to="/skills" className="a-btn" style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 28, padding: "0 10px", border: "1px solid var(--border-strong, var(--border))", borderRadius: "var(--radius)", color: "var(--text)", textDecoration: "none", fontSize: 12, fontWeight: 500 }}><I.external size={13} />Manage organization skills</Link>}
      </div>
      {!canManage && <div style={{ marginBottom: 14, padding: "9px 12px", fontSize: 12.5, color: "var(--text-dim)", border: "1px solid var(--border)", borderRadius: "var(--radius)" }}>Only administrators can attach or detach organization skills.</div>}
      {error && <div role="alert" style={{ marginBottom: 14, padding: "10px 12px", border: "1px solid color-mix(in srgb, var(--err) 35%, transparent)", background: "var(--err-bg)", color: "var(--err)", borderRadius: "var(--radius)", fontSize: 12.5 }}>{error}</div>}
      <div aria-live="polite" style={{ minHeight: 18, marginBottom: 8, color: "var(--ok)", fontSize: 12.5 }}>{notice}</div>

      <Card style={{ padding: 0, marginBottom: 20 }}>
        <div style={{ padding: "14px 16px" }}><strong style={{ fontSize: 14 }}>Attached to this project</strong><span style={{ marginLeft: 8, color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 12 }}>{attached.length}</span></div>
        {loading ? <div style={{ padding: "18px 16px", color: "var(--text-dim)", fontSize: 13 }}>Loading attached skills…</div>
          : attached.length === 0 ? <div style={{ padding: "18px 16px", borderTop: "1px solid var(--border)", color: "var(--text-dim)", fontSize: 13 }}>No skills are attached yet.</div>
          : attached.map((skill) => <SkillRow key={skill.id} skill={skill} attached busy={busy === skill.id} canManage={canManage} onAttach={setAttachment} />)}
      </Card>

      {canManage && <Card style={{ padding: 0 }}>
        <div style={{ padding: "14px 16px", display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <div style={{ flex: 1, minWidth: 200 }}><Input full size="sm" icon={<I.search />} placeholder="Search organization catalog…" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)" }}>{matchingSkills.length} available</span>
        </div>
        {!loading && matchingSkills.length === 0 ? <div style={{ padding: "18px 16px", borderTop: "1px solid var(--border)", color: "var(--text-dim)", fontSize: 13 }}>No organization skills match this search.</div>
          : matchingSkills.map((skill) => <SkillRow key={skill.id} skill={skill} attached={attachedIds.has(skill.id)} busy={busy === skill.id} canManage onAttach={setAttachment} />)}
      </Card>}
    </section>
  );
}
