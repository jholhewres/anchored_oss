import React, { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Card, Badge, Status, Btn, Input, Tabs } from "@/ds/components";
import { I } from "@/ds/icons";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Project, Memory, Triple, ChatAnswer, ProjectCategory } from "@/lib/types";
import { PROJECT_CATEGORIES, PROJECT_CATEGORY_LABELS } from "@/lib/types";
import { GraphView } from "@/components/GraphView";

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* fall through */ }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "0";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, text.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

function CommandBox({ cmd }: { cmd: string }) {
  return (
    <div style={{
      display: "flex", alignItems: "stretch",
      background: "var(--bg-2)", border: "1px solid var(--border)",
      borderRadius: "var(--radius)", overflow: "hidden",
    }}>
      <code style={{
        flex: 1, padding: "9px 12px",
        fontFamily: "var(--font-mono)", fontSize: 12.5, color: "var(--text)",
        whiteSpace: "nowrap", overflowX: "auto",
      }}>
        <span style={{ color: "var(--accent)" }}>$</span> {cmd}
      </code>
      <CopyButton text={cmd} />
    </div>
  );
}

function CopyButton({ text, label = "copy", inline }: { text: string; label?: string; inline?: boolean }) {
  const [state, setState] = useState<"idle" | "ok" | "err">("idle");
  async function onClick() {
    const ok = await copyToClipboard(text);
    setState(ok ? "ok" : "err");
    setTimeout(() => setState("idle"), 1800);
  }
  const color = state === "ok" ? "var(--ok)" : state === "err" ? "var(--err)" : "var(--text-dim)";
  return (
    <button type="button" onClick={onClick} style={{
      background: "transparent",
      border: inline ? "1px solid var(--border)" : 0,
      borderLeft: inline ? "1px solid var(--border)" : "1px solid var(--border)",
      borderRadius: inline ? "var(--radius)" : 0,
      color, cursor: "pointer",
      padding: inline ? "4px 10px" : "10px 14px",
      fontFamily: "var(--font-mono)", fontSize: 11,
      display: "inline-flex", alignItems: "center", gap: 5,
      alignSelf: "stretch", transition: "color .15s",
    }}>
      {state === "ok" ? <I.check size={12} /> : state === "err" ? <I.x size={12} /> : <I.copy size={12} />}
      {state === "ok" ? "copied!" : state === "err" ? "failed" : label}
    </button>
  );
}

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

function MetaField({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={{ display: "flex", gap: 12, padding: "8px 0", borderTop: "1px solid color-mix(in srgb, var(--border) 50%, transparent)" }}>
      <span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)", minWidth: 120, flex: "none" }}>{label}</span>
      <span style={{ fontSize: 12.5, color: "var(--text-muted)", wordBreak: "break-word" }}>{value}</span>
    </div>
  );
}

// MemoryDetail is a right-side drawer showing a memory's full content plus its
// curation/lifecycle metadata (status, quality, scope) so admins can see why a
// memory was kept, demoted, or flagged as a near-duplicate.
function MemoryDetail({ memory, onClose }: { memory: Memory; onClose: () => void }) {
  const meta = (memory.metadata ?? {}) as Record<string, unknown>;
  const str = (k: string): string | undefined => {
    const v = meta[k];
    return v == null ? undefined : String(v);
  };
  const status = str("curation_status");
  const statusTone = status === "low_signal" || status === "near_duplicate" ? "warn" : status ? "ok" : "neutral";
  const known = new Set(["curation_status", "curation_rule", "quality_score", "scope", "pinned", "canonical_of", "memory_type", "origin", "kind"]);
  const extra = Object.keys(meta).filter(k => !known.has(k));

  return (
    <div
      onClick={onClose}
      style={{ position: "fixed", inset: 0, background: "color-mix(in srgb, black 55%, transparent)", zIndex: 50, display: "flex", justifyContent: "flex-end" }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{ width: "min(560px, 92vw)", height: "100%", background: "var(--bg-1)", borderLeft: "1px solid var(--border)", overflowY: "auto", padding: "24px 26px" }}
      >
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 18 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <Badge tone={categoryTones[memory.category] || "neutral"}>{memory.category}</Badge>
            {status && <Badge tone={statusTone as "ok" | "warn" | "neutral"}>{status}</Badge>}
          </div>
          <button onClick={onClose} style={{ background: "transparent", border: 0, color: "var(--text-dim)", cursor: "pointer", padding: 6 }} aria-label="Close">
            <I.x size={16} />
          </button>
        </div>

        <div style={{ fontSize: 14.5, lineHeight: 1.6, color: "var(--text)", whiteSpace: "pre-wrap", marginBottom: 20 }}>
          {memory.content}
        </div>

        <div style={{ marginBottom: 8 }}>
          <MetaField label="id" value={<span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5 }}>{memory.id}</span>} />
          <MetaField label="author" value={memory.author_name || "unknown"} />
          <MetaField label="created" value={`${timeAgo(memory.created_at)} (${new Date(memory.created_at).toLocaleString()})`} />
          <MetaField label="updated" value={timeAgo(memory.updated_at)} />
          {memory.source && <MetaField label="source" value={memory.source} />}
          {memory.keywords && memory.keywords.length > 0 && <MetaField label="keywords" value={memory.keywords.join(", ")} />}
          {str("quality_score") && <MetaField label="quality_score" value={str("quality_score")} />}
          {str("scope") && <MetaField label="scope" value={str("scope")} />}
          {str("pinned") && <MetaField label="pinned" value={str("pinned")} />}
          {str("canonical_of") && <MetaField label="canonical_of" value={<span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5 }}>{str("canonical_of")}</span>} />}
          {str("curation_rule") && <MetaField label="curation_rule" value={str("curation_rule")} />}
          {extra.length > 0 && (
            <MetaField label="metadata" value={<code style={{ fontSize: 11, color: "var(--text-dim)" }}>{JSON.stringify(Object.fromEntries(extra.map(k => [k, meta[k]])))}</code>} />
          )}
        </div>
      </div>
    </div>
  );
}

// ChatTab is the optional RAG chat panel. It self-hides its input when the
// server reports chat disabled, so the feature stays opt-in.
function ChatTab({ projectId }: { projectId: string }) {
  const [enabled, setEnabled] = React.useState<boolean | null>(null);
  const [model, setModel] = React.useState("");
  const [q, setQ] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [ans, setAns] = React.useState<ChatAnswer | null>(null);
  const [err, setErr] = React.useState<string | null>(null);

  React.useEffect(() => {
    api.getChatStatus().then(s => { setEnabled(s.enabled); setModel(s.model); }).catch(() => setEnabled(false));
  }, []);

  const ask = () => {
    const query = q.trim();
    if (!query) return;
    setBusy(true); setErr(null); setAns(null);
    api.chat(projectId, query)
      .then(setAns)
      .catch((e: unknown) => setErr(e instanceof Error ? e.message : "chat failed"))
      .finally(() => setBusy(false));
  };

  if (enabled === null) return <div style={{ color: "var(--text-dim)", padding: 20 }}>Loading…</div>;
  if (!enabled) {
    return (
      <Card style={{ padding: "32px 22px", textAlign: "center" }}>
        <I.brain size={22} />
        <div style={{ fontSize: 14, fontWeight: 500, marginTop: 10 }}>Chat is not enabled</div>
        <div style={{ fontSize: 12.5, color: "var(--text-dim)", marginTop: 6, maxWidth: 420, marginInline: "auto" }}>
          An admin can enable the optional AI chat by configuring an LLM provider (OpenAI, z.ai, Anthropic, OpenRouter) in the server config. It answers using this project's memories.
        </div>
      </Card>
    );
  }

  return (
    <div>
      <div style={{ display: "flex", gap: 10, marginBottom: 14 }}>
        <Input full size="sm" placeholder="ask about this project's memories…" value={q}
          onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === "Enter") ask(); }} />
        <Btn variant="primary" size="sm" onClick={ask} disabled={busy}>{busy ? "Thinking…" : "Ask"}</Btn>
      </div>
      {model && <div style={{ fontSize: 11, color: "var(--text-dim)", marginBottom: 12, fontFamily: "var(--font-mono)" }}>model: {model}</div>}
      {err && <Card style={{ padding: 16, color: "var(--err)", fontSize: 13 }}>{err}</Card>}
      {ans && (
        <Card style={{ padding: 18 }}>
          <div style={{ fontSize: 14, lineHeight: 1.6, whiteSpace: "pre-wrap" }}>{ans.answer}</div>
          {ans.sources.length > 0 && (
            <div style={{ marginTop: 16, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
              <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginBottom: 8, fontFamily: "var(--font-mono)" }}>sources</div>
              {ans.sources.map((s, i) => (
                <div key={s.id} style={{ display: "flex", gap: 8, padding: "5px 0", fontSize: 12.5 }}>
                  <span style={{ color: "var(--text-dim)", fontFamily: "var(--font-mono)" }}>[{i + 1}]</span>
                  <Badge tone={categoryTones[s.category] || "neutral"}>{s.category}</Badge>
                  <span style={{ color: "var(--text-muted)" }}>{s.snippet}</span>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}
    </div>
  );
}

function ConnectTab({ project, onGoToSettings }: { project: Project; onGoToSettings: () => void }) {
  const { me } = useAuth();
  const origin = window.location.origin;
  // Naming the remote after the org slug keeps multi-server CLI setups
  // (personal + company) unambiguous from the very first command.
  const remoteName = me?.org_slug || "team";
  const configureCmd = `anchored remote configure --server ${origin} --key <your-api-key> --name ${remoteName}`;
  const linkCmd = `anchored remote link ${project.id} --remote ${remoteName}`;
  const syncCmd = `anchored remote sync --remote ${remoteName}`;

  const fullScript = [
    `# 1. Connect to ${origin}`,
    configureCmd,
    `# 2. Link project: ${project.name}`,
    linkCmd,
    `# 3. Sync memories`,
    syncCmd,
  ].join("\n");

  return (
    <div style={{ maxWidth: 640 }}>
      <div style={{ fontSize: 16, fontWeight: 500, marginBottom: 6 }}>Connect your CLI to this project</div>
      <div style={{ fontSize: 13, color: "var(--text-muted)", lineHeight: 1.55, marginBottom: 22 }}>
        Run these commands on your dev machine to sync local memories with <strong>{project.name}</strong>.
        You need an API key — create one in the <a href="/api-keys" style={{ color: "var(--accent)" }}>API Keys</a> page if you don't have one.
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
        <div>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase", marginBottom: 6 }}>1. Connect to this server</div>
          <CommandBox cmd={configureCmd} />
        </div>

        <div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" }}>2. Link this project</span>
            <Badge tone="outline">{project.name}</Badge>
          </div>
          <CommandBox cmd={linkCmd} />
          <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginTop: 6, lineHeight: 1.5 }}>
            Client v0.6.9+ also accepts the slug:{" "}
            <code style={{ fontFamily: "var(--font-mono)", color: "var(--text-muted)" }}>
              anchored remote link {project.slug} --remote {remoteName}
            </code>
          </div>
        </div>

        <div>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase", marginBottom: 6 }}>3. Sync memories</div>
          <CommandBox cmd={syncCmd} />
          <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginTop: 6, lineHeight: 1.5 }}>
            Pushes local memories up and pulls team memories down. Uses the linked project from step 2.
          </div>
        </div>
      </div>

      {project.repo_url ? (
        <div style={{
          marginTop: 20, padding: "12px 14px",
          background: "var(--ok-bg)", border: "1px solid color-mix(in srgb, var(--ok) 25%, transparent)",
          borderRadius: "var(--radius)", fontSize: 12.5, color: "var(--text-muted)", lineHeight: 1.55,
        }}>
          <strong style={{ color: "var(--ok)" }}>Auto-routing enabled.</strong>{" "}
          Any clone with git origin <code style={{ fontFamily: "var(--font-mono)" }}>{project.repo_url}</code> will
          automatically route syncs to this project — no explicit link needed.
        </div>
      ) : (
        <div style={{
          marginTop: 20, padding: "12px 14px",
          background: "var(--warn-bg)", border: "1px solid color-mix(in srgb, var(--warn) 25%, transparent)",
          borderRadius: "var(--radius)", fontSize: 12.5, color: "var(--text-muted)", lineHeight: 1.55,
        }}>
          <strong style={{ color: "var(--warn)" }}>No repository URL set.</strong>{" "}
          Syncs route by git origin — set the Repository URL in the{" "}
          <button
            type="button"
            onClick={onGoToSettings}
            style={{ background: "none", border: "none", padding: 0, color: "var(--accent)", cursor: "pointer", fontSize: "inherit", fontFamily: "inherit" }}
          >
            Settings
          </button>{" "}
          tab so your team's clones resolve to this project automatically.
        </div>
      )}

      <div style={{ marginTop: 24, display: "flex", alignItems: "center", gap: 12 }}>
        <CopyButton text={fullScript} label="copy all commands" inline />
      </div>
    </div>
  );
}

function SettingsTab({ project, onUpdated }: { project: Project; onUpdated: (p: Project) => void }) {
  const { me } = useAuth();
  const navigate = useNavigate();
  const isAdmin = me?.scope === "admin";

  const [name, setName] = React.useState(project.name);
  const [slug, setSlug] = React.useState(project.slug);
  const [repoUrl, setRepoUrl] = React.useState(project.repo_url ?? "");
  const [category, setCategory] = React.useState<ProjectCategory>(project.category);
  const [saving, setSaving] = React.useState(false);
  const [saveState, setSaveState] = React.useState<"idle" | "saved" | "error">("idle");
  const [saveError, setSaveError] = React.useState<string | null>(null);

  // Danger zone
  const [confirmSlug, setConfirmSlug] = React.useState("");
  const [deleting, setDeleting] = React.useState(false);
  const [deleteError, setDeleteError] = React.useState<string | null>(null);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    const patch: Record<string, string> = {};
    if (name !== project.name) patch.name = name;
    if (slug !== project.slug) patch.slug = slug;
    if (repoUrl !== (project.repo_url ?? "")) patch.repo_url = repoUrl;
    if (category !== project.category) patch.category = category;
    if (Object.keys(patch).length === 0) {
      setSaveState("saved");
      setTimeout(() => setSaveState("idle"), 1800);
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      const updated = await api.patchProject(project.id, patch as { name?: string; slug?: string; repo_url?: string; category?: ProjectCategory });
      onUpdated(updated);
      setSaveState("saved");
      setTimeout(() => setSaveState("idle"), 2500);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Save failed";
      setSaveError(msg);
      setSaveState("error");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (confirmSlug !== project.slug) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await api.deleteProject(project.id);
      navigate("/projects");
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Delete failed";
      setDeleteError(msg);
      setDeleting(false);
    }
  }

  return (
    <div style={{ maxWidth: 600 }}>
      {/* Edit form */}
      <Card style={{ padding: 24, marginBottom: 20 }}>
        <div style={{ fontSize: 15, fontWeight: 500, marginBottom: 18 }}>Project settings</div>
        {!isAdmin && (
          <div style={{
            padding: "10px 14px", marginBottom: 18,
            background: "var(--bg-3)", border: "1px solid var(--border)",
            borderRadius: "var(--radius)", fontSize: 12.5, color: "var(--text-dim)",
          }}>
            Only admins can edit project settings.
          </div>
        )}
        <form onSubmit={handleSave}>
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>Name</div>
              <Input full size="md" placeholder="my-service" value={name}
                onChange={e => setName(e.target.value)} disabled={!isAdmin} />
            </div>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>Slug</div>
              <Input full size="md" placeholder="my-service" value={slug} mono
                onChange={e => setSlug(e.target.value)} disabled={!isAdmin} />
              <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginTop: 4 }}>
                Allowed characters: a–z, 0–9, hyphen (a-z0-9-)
              </div>
            </div>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>Repository URL</div>
              <Input full size="md" placeholder="git@github.com:org/repo.git" value={repoUrl} mono
                onChange={e => setRepoUrl(e.target.value)} disabled={!isAdmin} />
              <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginTop: 4, lineHeight: 1.5 }}>
                Git remote URL (SSH or HTTPS). Used to route <code style={{ fontFamily: "var(--font-mono)" }}>anchored remote sync</code> by git origin.
              </div>
            </div>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>Category</div>
              <select
                value={category}
                onChange={e => setCategory(e.target.value as ProjectCategory)}
                disabled={!isAdmin}
                style={{
                  width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
                  background: "var(--bg-input)", border: "1px solid var(--border)",
                  borderRadius: "var(--radius)", color: "var(--text)", cursor: isAdmin ? "pointer" : "not-allowed",
                  opacity: isAdmin ? 1 : 0.6,
                }}
              >
                {PROJECT_CATEGORIES.map(c => (
                  <option key={c} value={c}>{PROJECT_CATEGORY_LABELS[c]}</option>
                ))}
              </select>
            </div>

            {saveError && (
              <div style={{ padding: "8px 12px", background: "var(--err-bg)", border: "1px solid color-mix(in srgb, var(--err) 25%, transparent)", borderRadius: "var(--radius)", fontSize: 12.5, color: "var(--err)" }}>
                {saveError}
              </div>
            )}

            {isAdmin && (
              <div style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 4 }}>
                <Btn variant="primary" size="md" type="submit" disabled={saving}>
                  {saving ? "Saving…" : "Save"}
                </Btn>
                {saveState === "saved" && (
                  <span style={{ fontSize: 12.5, color: "var(--ok)", fontFamily: "var(--font-mono)", display: "inline-flex", alignItems: "center", gap: 5 }}>
                    <I.check size={13} /> Saved
                  </span>
                )}
              </div>
            )}
          </div>
        </form>
      </Card>

      {/* Read-only key info */}
      <Card style={{ padding: 20, marginBottom: 20 }}>
        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 4 }}>Routing keys</div>
        <div style={{ fontSize: 12, color: "var(--text-dim)", marginBottom: 14, lineHeight: 1.5 }}>
          Keys are derived from the repository URL and used to route{" "}
          <code style={{ fontFamily: "var(--font-mono)" }}>anchored remote sync</code> by git origin.
          Canonical key uses the current normalization algorithm; legacy key preserves v1 behaviour for older clients.
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          <div style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", minWidth: 110, paddingTop: 1 }}>remote_key</span>
            <code style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-muted)", wordBreak: "break-all" }}>{project.remote_key || "—"}</code>
          </div>
          {project.remote_key_v1 && (
            <div style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", minWidth: 110, paddingTop: 1 }}>remote_key_v1</span>
              <code style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-muted)", wordBreak: "break-all" }}>{project.remote_key_v1}</code>
            </div>
          )}
        </div>
      </Card>

      {/* Danger zone — admins only */}
      {isAdmin && (
        <Card style={{ padding: 20, border: "1px solid color-mix(in srgb, var(--err) 30%, transparent)" }}>
          <div style={{ fontSize: 13, fontWeight: 500, color: "var(--err)", marginBottom: 6 }}>Danger zone</div>
          <div style={{ fontSize: 12.5, color: "var(--text-muted)", marginBottom: 14, lineHeight: 1.5 }}>
            Deleting a project is permanent. All memories and settings will be removed.
            Type the project slug <code style={{ fontFamily: "var(--font-mono)" }}>{project.slug}</code> to confirm.
          </div>
          <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
            <Input
              size="md"
              placeholder={project.slug}
              value={confirmSlug}
              onChange={e => setConfirmSlug(e.target.value)}
              mono
              style={{ width: 200 }}
            />
            <Btn
              variant="danger"
              size="md"
              onClick={handleDelete}
              disabled={confirmSlug !== project.slug || deleting}
            >
              {deleting ? "Deleting…" : "Delete project"}
            </Btn>
          </div>
          {deleteError && (
            <div style={{ marginTop: 10, fontSize: 12.5, color: "var(--err)" }}>{deleteError}</div>
          )}
        </Card>
      )}
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
  const [graphPred, setGraphPred] = React.useState("");
  const [graphSearch, setGraphSearch] = React.useState("");
  const [searchMode, setSearchMode] = React.useState<"text" | "semantic">("text");
  const [results, setResults] = React.useState<Memory[] | null>(null);
  const [searching, setSearching] = React.useState(false);
  const [selected, setSelected] = React.useState<Memory | null>(null);
  const [categoryFilter, setCategoryFilter] = React.useState<string>("");

  // Reset offset when category changes to avoid empty pages.
  React.useEffect(() => { setOffset(0); }, [categoryFilter]);

  const runSearch = React.useCallback(() => {
    if (!id) return;
    const term = search.trim();
    if (!term) { setResults(null); return; }
    setSearching(true);
    api.searchMemories(id, term, searchMode, 50)
      .then(setResults)
      .catch(() => setResults([]))
      .finally(() => setSearching(false));
  }, [id, search, searchMode]);

  React.useEffect(() => {
    if (!id) return;
    Promise.all([
      api.getProject(id),
      api.getProjectMemories(id, 20, 0, categoryFilter || undefined),
    ])
      .then(([p, m]) => {
        setProject(p);
        setMemories(m.memories);
        setMemTotal(m.total);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [id, categoryFilter]);

  React.useEffect(() => {
    if (!id || offset === 0) return;
    api.getProjectMemories(id, 20, offset, categoryFilter || undefined)
      .then(m => { setMemories(m.memories); setMemTotal(m.total); })
      .catch(() => {});
  }, [id, offset, categoryFilter]);

  React.useEffect(() => {
    if (!id || activeTab !== "graph") return;
    api.getProjectGraph(id, 200, 0)
      .then(r => { setTriples(r.triples); setTripleTotal(r.total); })
      .catch(() => {});
  }, [id, activeTab]);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;
  if (!project) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Project not found.</div>;

  // Server results take precedence; until a search is submitted, fall back to
  // a client-side substring filter over the loaded page for responsiveness.
  const displayed = results ?? (search
    ? memories.filter(m => m.content.toLowerCase().includes(search.toLowerCase()) || m.category.toLowerCase().includes(search.toLowerCase()))
    : memories);

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
          { key: "chat", label: "Chat", icon: <I.brain /> },
          { key: "connect", label: "Connect", icon: <I.terminal /> },
          { key: "settings", label: "Settings", icon: <I.settings /> },
        ]}
      />

      {activeTab === "memories" && (
        <div style={{ marginTop: 22 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
            <Input
              icon={<I.search />}
              placeholder={searchMode === "semantic" ? "search by meaning..." : "search memories..."}
              size="sm"
              style={{ flex: 1, maxWidth: 420 }}
              value={search}
              onChange={e => setSearch(e.target.value)}
              onKeyDown={e => { if (e.key === "Enter") runSearch(); }}
            />
            <div style={{ display: "inline-flex", border: "1px solid var(--border)", borderRadius: "var(--radius)", overflow: "hidden" }}>
              <Btn variant={searchMode === "text" ? "outline" : "ghost"} size="sm" onClick={() => setSearchMode("text")}>Text</Btn>
              <Btn variant={searchMode === "semantic" ? "outline" : "ghost"} size="sm" onClick={() => setSearchMode("semantic")}>Semantic</Btn>
            </div>
            <Btn variant="primary" size="sm" onClick={runSearch} disabled={searching}>{searching ? "Searching..." : "Search"}</Btn>
            {results !== null && (
              <Btn variant="ghost" size="sm" onClick={() => { setResults(null); setSearch(""); }}>Clear</Btn>
            )}
            <div style={{ flex: 1 }} />
            <Btn variant="ghost" size="sm" iconR={<I.chevD />}>Newest first</Btn>
          </div>

          <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 14, flexWrap: "wrap" }}>
            <span style={{ fontSize: 11.5, color: "var(--text-dim)", fontFamily: "var(--font-mono)", marginRight: 4 }}>category:</span>
            <button onClick={() => setCategoryFilter("")} style={{
              padding: "3px 10px", fontSize: 11.5, fontWeight: 500, cursor: "pointer", border: "1px solid transparent",
              borderRadius: "var(--radius)", background: categoryFilter === "" ? "var(--accent-bg)" : "transparent",
              color: categoryFilter === "" ? "var(--accent)" : "var(--text-dim)", transition: "background .12s, color .12s",
            }}>all</button>
            {(["decision", "fact", "learning", "plan", "summary", "event", "preference"] as const).map(cat => (
              <button key={cat} onClick={() => setCategoryFilter(categoryFilter === cat ? "" : cat)} style={{
                padding: "3px 10px", fontSize: 11.5, fontWeight: 500, cursor: "pointer",
                border: categoryFilter === cat ? `1px solid var(--accent-border)` : "1px solid transparent",
                borderRadius: "var(--radius)",
                background: categoryFilter === cat ? "var(--accent-bg)" : "transparent",
                color: categoryFilter === cat ? "var(--accent)" : "var(--text-dim)",
                transition: "background .12s, color .12s, border-color .12s",
              }}>{cat}</button>
            ))}
          </div>

          {displayed.length === 0 ? (
            <Card style={{ padding: "40px 22px", textAlign: "center" }}>
              <div style={{ fontSize: 13, color: "var(--text-dim)" }}>
                {results !== null ? "No matches for this search." : "No memories yet."}
              </div>
            </Card>
          ) : (
            <Card style={{ padding: 0 }}>
              {displayed.map((m, i, arr) => (
                <div key={m.id} onClick={() => setSelected(m)} style={{
                  padding: "18px 22px", borderBottom: i < arr.length - 1 ? "1px solid var(--border)" : "none",
                  display: "grid", gridTemplateColumns: "110px 1fr 80px 40px", gap: 18, alignItems: "flex-start", cursor: "pointer",
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
                  <button onClick={e => { e.stopPropagation(); setSelected(m); }} style={{ background: "transparent", border: 0, color: "var(--text-dim)", padding: 6, cursor: "pointer", borderRadius: 4 }}>
                    <I.more size={14} />
                  </button>
                </div>
              ))}
            </Card>
          )}

          <div style={{ marginTop: 14, display: "flex", justifyContent: "space-between", alignItems: "center", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
            <span>
              {results !== null
                ? `${displayed.length} ${searchMode} result${displayed.length === 1 ? "" : "s"}`
                : `showing ${displayed.length} of ${memTotal.toLocaleString()} · sorted by recency`}
            </span>
            {results === null && (
              <div style={{ display: "flex", gap: 8 }}>
                <Btn variant="ghost" size="sm" style={offset === 0 ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setOffset(Math.max(0, offset - 20))}>Previous</Btn>
                <Btn variant="ghost" size="sm" style={offset + 20 >= memTotal ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setOffset(offset + 20)}>Load more</Btn>
              </div>
            )}
          </div>
        </div>
      )}

      {selected && <MemoryDetail memory={selected} onClose={() => setSelected(null)} />}

      {activeTab === "graph" && (() => {
        const predicates = Array.from(new Set(triples.map(t => t.predicate))).sort();
        const gq = graphSearch.trim().toLowerCase();
        const filteredTriples = triples.filter(t =>
          (!graphPred || t.predicate === graphPred) &&
          (!gq || t.subject.toLowerCase().includes(gq) || t.object.toLowerCase().includes(gq))
        );
        return (
          <div style={{ marginTop: 22 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14, flexWrap: "wrap" }}>
              <Input icon={<I.search />} placeholder="filter nodes (subject/object)…" size="sm" style={{ width: 280 }}
                value={graphSearch} onChange={e => setGraphSearch(e.target.value)} />
              <select value={graphPred} onChange={e => setGraphPred(e.target.value)} style={{
                height: 28, padding: "0 8px", fontSize: 12, background: "var(--bg-2)", color: "var(--text)",
                border: "1px solid var(--border)", borderRadius: "var(--radius)", fontFamily: "var(--font-mono)", cursor: "pointer",
              }}>
                <option value="">predicate: all</option>
                {predicates.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
              {(graphPred || gq) && <Btn variant="ghost" size="sm" onClick={() => { setGraphPred(""); setGraphSearch(""); }}>Clear</Btn>}
              <div style={{ flex: 1 }} />
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)" }}>
                {filteredTriples.length} of {tripleTotal} edges{tripleTotal > triples.length ? ` (showing first ${triples.length})` : ""}
              </span>
            </div>
            <GraphView triples={filteredTriples} total={tripleTotal} />
          </div>
        );
      })()}

      {activeTab === "chat" && id && (
        <div style={{ marginTop: 22 }}>
          <ChatTab projectId={id} />
        </div>
      )}

      {activeTab === "connect" && (
        <div style={{ marginTop: 22 }}>
          <ConnectTab project={project} onGoToSettings={() => setActiveTab("settings")} />
        </div>
      )}

      {activeTab === "settings" && (
        <div style={{ marginTop: 22 }}>
          <SettingsTab project={project} onUpdated={setProject} />
        </div>
      )}
    </div>
  );
}
