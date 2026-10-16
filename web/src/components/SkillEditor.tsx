import React, { useEffect, useRef, useState } from "react";

import { Badge, Btn, Input } from "@/ds/components";
import { I } from "@/ds/icons";
import { ApiError, api } from "@/lib/api";
import type { Skill, SkillHeader, SkillStatus, SkillTemplate as ApiSkillTemplate } from "@/lib/types";

export type CatalogSkill = Skill;
export type SkillTemplate = ApiSkillTemplate;

const writableStatuses: SkillStatus[] = ["draft", "active", "deprecated"];

const fieldStyle: React.CSSProperties = {
  width: "100%",
  minHeight: 34,
  boxSizing: "border-box",
  padding: "8px 10px",
  color: "var(--text)",
  background: "var(--bg-input)",
  border: "1px solid var(--border)",
  borderRadius: "var(--radius)",
  fontFamily: "inherit",
  fontSize: 13,
};

function formatError(error: unknown, fallback: string) {
  return error instanceof ApiError || error instanceof Error ? error.message : fallback;
}

function stableSlug(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function skillHeader(name: string, description: string): SkillHeader {
  return { name: stableSlug(name), description: description.trim() };
}

function yamlScalar(value: string) {
  const plain = value.trim() === value && value !== "" && !value.includes("\n") && !value.includes(": ") && !value.includes(" #");
  return plain ? value : JSON.stringify(value);
}

function skillHeaderText(header: SkillHeader) {
  return `---\nname: ${header.name}\ndescription: ${yamlScalar(header.description)}\n---`;
}

const skillHeaderPlaceholder = `---
name: reviewing-pull-requests
description: Review pull requests for correctness, tests, and project conventions.
---`;

function parseYamlScalar(raw: string, field: string) {
  const value = raw.trim();
  if (!value) return "";
  if (value.startsWith('"')) {
    try {
      const parsed = JSON.parse(value);
      if (typeof parsed !== "string") throw new Error();
      return parsed;
    } catch {
      throw new Error(`${field} must be a valid YAML string.`);
    }
  }
  if (value.startsWith("'")) {
    if (!value.endsWith("'") || value.length < 2) throw new Error(`${field} must be a valid YAML string.`);
    return value.slice(1, -1).replace(/''/g, "'");
  }
  return value;
}

function parseSkillHeader(value: string): SkillHeader {
  const lines = value.replace(/\r\n?/g, "\n").trim().split("\n");
  if (lines.length < 4 || lines[0] !== "---" || lines[lines.length - 1] !== "---") {
    throw new Error("Frontmatter must start and end with ---.");
  }

  const fields: Partial<SkillHeader> = {};
  for (const line of lines.slice(1, -1)) {
    if (!line.trim()) continue;
    const match = line.match(/^([A-Za-z][A-Za-z0-9_-]*):\s*(.*)$/);
    if (!match) throw new Error(`Invalid frontmatter line: ${line.trim()}`);
    const key = match[1];
    if (key !== "name" && key !== "description") {
      throw new Error("Frontmatter can contain only name and description.");
    }
    if (fields[key] !== undefined) throw new Error(`Frontmatter contains duplicate ${key}.`);
    fields[key] = parseYamlScalar(match[2], key);
  }

  if (!fields.name || !/^[a-z0-9][a-z0-9-]{0,63}$/.test(fields.name)) {
    throw new Error("Frontmatter name must use lowercase letters, numbers, and hyphens (maximum 64 characters).");
  }
  if (!fields.description?.trim()) throw new Error("Frontmatter description is required.");
  return { name: fields.name, description: fields.description.trim() };
}

function initialSkillHeader(skill: CatalogSkill | null, template: SkillTemplate | null) {
  if (skill) return skillHeaderText(skill.frontmatter ?? skillHeader(skill.slug, skill.purpose));
  if (template) return skillHeaderText(template.frontmatter ?? skillHeader(template.slug, template.purpose));
  return "";
}

interface SkillEditorProps {
  skill?: CatalogSkill | null;
  template?: SkillTemplate | null;
  onClose: () => void;
  onSaved: (skill: CatalogSkill) => void;
}

/** An organization-skill editor: SKILL.md header, body, then server-rendered runtime preview. */
export function SkillEditor({ skill: initialSkill = null, template = null, onClose, onSaved }: SkillEditorProps) {
  const [savedSkill, setSavedSkill] = useState<CatalogSkill | null>(initialSkill);
  const currentSkill = savedSkill ?? initialSkill;
  const editing = Boolean(currentSkill);
  const [name, setName] = useState(initialSkill?.name ?? template?.name ?? "");
  const [frontmatterText, setFrontmatterText] = useState(() => initialSkillHeader(initialSkill, template));
  const [body, setBody] = useState(initialSkill?.body ?? template?.body ?? "");
  const [status, setStatus] = useState<SkillStatus>(initialSkill?.status ?? "draft");
  const [canonicalPreview, setCanonicalPreview] = useState(initialSkill?.content ?? "");
  const [loading, setLoading] = useState(Boolean(initialSkill && !initialSkill.content));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const triggerRef = useRef<HTMLElement | null>(document.activeElement as HTMLElement | null);
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    dialogRef.current?.querySelector<HTMLElement>("input:not([disabled])")?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = dialogRef.current.querySelectorAll<HTMLElement>("button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])");
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    window.addEventListener("keydown", keydown);
    return () => {
      window.removeEventListener("keydown", keydown);
      triggerRef.current?.focus();
    };
  }, [onClose]);

  useEffect(() => {
    if (!initialSkill || initialSkill.content) return;
    api.getSkill(initialSkill.id)
      .then((response) => {
        setSavedSkill(response);
        setName(response.name);
        setFrontmatterText(skillHeaderText(response.frontmatter ?? skillHeader(response.slug, response.purpose)));
        setBody(response.body ?? "");
        setStatus(response.status);
        setCanonicalPreview(response.content ?? "");
      })
      .catch((reason) => setError(formatError(reason, "Could not load this skill.")))
      .finally(() => setLoading(false));
  }, [initialSkill]);

  const attachedProjects = currentSkill?.project_count ?? 0;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!name.trim() || !frontmatterText.trim() || !body.trim()) {
      setError("Display name, frontmatter, and Markdown instructions are required.");
      return;
    }
    let frontmatter: SkillHeader;
    try {
      frontmatter = parseSkillHeader(frontmatterText);
    } catch (reason) {
      setError(formatError(reason, "Frontmatter is invalid."));
      return;
    }
    if (currentSkill && frontmatter.name !== currentSkill.slug) {
      setError("The frontmatter name cannot change after the skill is created.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const payload = { name: name.trim(), slug: frontmatter.name, purpose: frontmatter.description, body, frontmatter, status };
      const saved = currentSkill
        ? await api.updateSkill(currentSkill.id, { name: payload.name, purpose: payload.purpose, body: payload.body, frontmatter: payload.frontmatter, status: payload.status })
        : await api.createSkill(payload);
      setSavedSkill(saved);
      setFrontmatterText(skillHeaderText(saved.frontmatter ?? skillHeader(saved.slug, saved.purpose)));
      setCanonicalPreview(saved.content ?? "");
      setBody(saved.body ?? body);
      onSaved(saved);
    } catch (reason) {
      setError(formatError(reason, "Could not publish this skill."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div role="presentation" onMouseDown={onClose} style={{ position: "fixed", inset: 0, zIndex: 70, display: "flex", justifyContent: "flex-end", background: "color-mix(in srgb, black 55%, transparent)" }}>
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby="skill-editor-title" onMouseDown={(event) => event.stopPropagation()} style={{ width: "min(780px, 100vw)", height: "100%", overflowY: "auto", padding: "24px", background: "var(--bg-1)", borderLeft: "1px solid var(--border)", boxSizing: "border-box" }}>
        <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center", marginBottom: 20 }}>
          <div>
            <h2 id="skill-editor-title" style={{ margin: 0, fontSize: 17 }}>{editing ? "Edit organization skill" : "Create organization skill"}</h2>
            {template && !editing && <div style={{ marginTop: 5, color: "var(--text-dim)", fontSize: 12.5 }}>Starting from built-in template <code>{template.id}@{template.revision}</code>.</div>}
          </div>
          <Btn variant="ghost" size="sm" onClick={onClose} aria-label="Close skill editor" icon={<I.x />} />
        </div>
        <form onSubmit={submit}>
          <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
            {error && <div role="alert" style={{ padding: "10px 12px", border: "1px solid color-mix(in srgb, var(--err) 35%, transparent)", background: "var(--err-bg)", color: "var(--err)", borderRadius: "var(--radius)", fontSize: 12.5 }}>{error}</div>}
            <section>
              <h3 style={{ margin: "0 0 10px", fontSize: 13.5 }}>Identity</h3>
              <label style={{ display: "grid", gap: 6, fontSize: 12.5 }}>Display name
                <Input full value={name} onChange={(event) => setName(event.target.value)} disabled={loading || saving} placeholder="Pull request reviewer" />
              </label>
            </section>
            <section>
              <h3 style={{ margin: "0 0 6px", fontSize: 13.5 }}>Skill header</h3>
              <p style={{ margin: "0 0 9px", fontSize: 12, color: "var(--text-dim)", lineHeight: 1.45 }}>Editable YAML frontmatter. It contains only <code>name</code> and <code>description</code>; the example below is a placeholder and is not saved.</p>
              <textarea value={frontmatterText} onChange={(event) => setFrontmatterText(event.target.value)} placeholder={skillHeaderPlaceholder} rows={5} spellCheck={false} style={{ ...fieldStyle, resize: "vertical", fontFamily: "var(--font-mono)", lineHeight: 1.55 }} disabled={loading || saving} />
              {editing && <div style={{ marginTop: 6, color: "var(--text-dim)", fontSize: 11.5 }}>The <code>name</code> is stable after creation; the <code>description</code> remains editable.</div>}
            </section>
            <section>
              <h3 style={{ margin: "0 0 6px", fontSize: 13.5 }}>Instructions</h3>
              <p style={{ margin: "0 0 9px", fontSize: 12, color: "var(--text-dim)" }}>Markdown body, stored independently from frontmatter.</p>
              <textarea value={body} onChange={(event) => setBody(event.target.value)} rows={14} style={{ ...fieldStyle, resize: "vertical", fontFamily: "var(--font-mono)", lineHeight: 1.5 }} disabled={loading || saving} />
            </section>
            <section>
              <h3 style={{ margin: "0 0 8px", fontSize: 13.5 }}>Publishing</h3>
              <label style={{ display: "grid", gap: 6, fontSize: 12.5, maxWidth: 240 }}>Lifecycle
                <select value={status} onChange={(event) => setStatus(event.target.value as SkillStatus)} style={fieldStyle} disabled={loading || saving}>
                  {writableStatuses.map((value) => <option key={value} value={value}>{value}</option>)}
                </select>
              </label>
              {editing && <div style={{ marginTop: 12, padding: "10px 12px", border: "1px solid var(--warn)", background: "var(--warn-bg)", borderRadius: "var(--radius)", fontSize: 12.5, lineHeight: 1.45 }}>
                <Badge tone="warn">Impact</Badge><span style={{ marginLeft: 8 }}>Publishing creates a new version and immediately affects {attachedProjects} attached {attachedProjects === 1 ? "project" : "projects"}.</span>
              </div>}
            </section>
            {canonicalPreview && <section>
              <h3 style={{ margin: "0 0 6px", fontSize: 13.5 }}>Canonical runtime preview</h3>
              <p style={{ margin: "0 0 9px", fontSize: 12, color: "var(--text-dim)", lineHeight: 1.45 }}>This is the exact <code>SKILL.md</code> returned by the server; the browser does not assemble frontmatter.</p>
              <pre style={{ margin: 0, padding: 12, overflowX: "auto", whiteSpace: "pre-wrap", background: "var(--bg-2)", border: "1px solid var(--border)", borderRadius: "var(--radius)", fontFamily: "var(--font-mono)", fontSize: 11.5, lineHeight: 1.55 }}>{canonicalPreview}</pre>
            </section>}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
              <Btn type="button" variant="outline" size="md" onClick={onClose} disabled={saving}>Close</Btn>
              <Btn variant="primary" size="md" disabled={saving || loading}>{saving ? "Publishing…" : editing ? "Publish changes" : "Create skill"}</Btn>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}
