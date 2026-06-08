import React from "react";
import { Card, Btn, Input, Badge } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import type { Guardrail, GuardrailKind, OrgPolicy } from "@/lib/types";

const KIND_META: Record<GuardrailKind, { label: string; tone: "err" | "warn" | "accent" | "info" | "neutral"; security: boolean }> = {
  secret_detection: { label: "Secret detection", tone: "err", security: true },
  local_path_redaction: { label: "Local path block", tone: "err", security: true },
  user_scope_block: { label: "User-scope block", tone: "warn", security: true },
  category: { label: "Category", tone: "accent", security: false },
  regex: { label: "Regex", tone: "info", security: false },
  keyword: { label: "Keyword", tone: "neutral", security: false },
};

// Toggle is a small on/off switch styled with CSS vars.
function Toggle({ on, onClick, disabled }: { on: boolean; onClick: () => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-pressed={on}
      style={{
        width: 38, height: 22, borderRadius: 11, position: "relative",
        background: on ? "var(--accent)" : "var(--bg-3, #2a2a2a)",
        border: "1px solid var(--border)", cursor: disabled ? "default" : "pointer",
        transition: "background 0.15s", flex: "none", opacity: disabled ? 0.5 : 1,
      }}
    >
      <span style={{
        position: "absolute", top: 2, left: on ? 18 : 2, width: 16, height: 16,
        borderRadius: 8, background: "#fff", transition: "left 0.15s",
      }} />
    </button>
  );
}

function GuardrailRow({ g, onChange }: { g: Guardrail; onChange: () => void }) {
  const toast = useToast();
  const meta = KIND_META[g.kind];
  const [editing, setEditing] = React.useState(false);
  const [label, setLabel] = React.useState(g.label);
  const [value, setValue] = React.useState(g.value);
  const [busy, setBusy] = React.useState(false);

  async function toggle() {
    setBusy(true);
    try {
      await api.updateGuardrail(g.id, { enabled: !g.enabled });
      onChange();
    } catch (e) {
      toast.push({ title: "Toggle failed", description: e instanceof Error ? e.message : "", variant: "error" });
    } finally {
      setBusy(false);
    }
  }

  async function saveEdit() {
    setBusy(true);
    try {
      await api.updateGuardrail(g.id, { label: label.trim(), value: value.trim() });
      setEditing(false);
      onChange();
    } catch (e) {
      toast.push({ title: "Save failed", description: e instanceof Error ? e.message : "", variant: "error" });
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await api.deleteGuardrail(g.id);
      onChange();
    } catch (e) {
      toast.push({ title: "Delete failed", description: e instanceof Error ? e.message : "", variant: "error" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: 14, padding: "14px 22px", borderBottom: "1px solid var(--border)", opacity: g.enabled ? 1 : 0.55 }}>
      <Toggle on={g.enabled} onClick={toggle} disabled={busy} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
          {editing ? (
            <Input size="sm" value={label} onChange={e => setLabel(e.target.value)} style={{ width: 220 }} />
          ) : (
            <span style={{ fontSize: 14, fontWeight: 500 }}>{g.label}</span>
          )}
          <Badge tone={meta.tone}>{meta.label}</Badge>
          {g.builtin && <Badge tone="outline">built-in</Badge>}
        </div>
        {g.description && !editing && (
          <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 4 }}>{g.description}</div>
        )}
        {!meta.security && (
          editing ? (
            <Input size="sm" value={value} onChange={e => setValue(e.target.value)} style={{ marginTop: 8, width: 320, fontFamily: "var(--font-mono)" }} placeholder={g.kind === "category" ? "category name" : "pattern"} />
          ) : (
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-muted)", marginTop: 4 }}>{g.value}</div>
          )
        )}
      </div>
      {/* Actions: builtins toggle only; custom rules can edit/delete. */}
      {!meta.security && (
        <div style={{ display: "flex", gap: 6, flex: "none" }}>
          {editing ? (
            <>
              <Btn size="sm" variant="primary" onClick={saveEdit} disabled={busy || !value.trim()}>Save</Btn>
              <Btn size="sm" variant="ghost" onClick={() => { setEditing(false); setLabel(g.label); setValue(g.value); }}>Cancel</Btn>
            </>
          ) : (
            <>
              <Btn size="sm" variant="ghost" onClick={() => setEditing(true)} title="Edit"><I.edit size={14} /></Btn>
              <Btn size="sm" variant="ghost" onClick={remove} disabled={busy} title="Delete"><I.trash size={14} /></Btn>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function AddGuardrail({ onCreated }: { onCreated: () => void }) {
  const toast = useToast();
  const [kind, setKind] = React.useState<GuardrailKind>("keyword");
  const [value, setValue] = React.useState("");
  const [label, setLabel] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  async function create() {
    if (!value.trim()) return;
    setBusy(true);
    try {
      await api.createGuardrail({ kind, value: value.trim(), label: label.trim() || undefined });
      setValue(""); setLabel("");
      onCreated();
      toast.push({ title: "Guardrail added", variant: "success" });
    } catch (e) {
      toast.push({ title: "Create failed", description: e instanceof Error ? e.message : "", variant: "error" });
    } finally {
      setBusy(false);
    }
  }

  const placeholder = kind === "category" ? "category to block (e.g. event)"
    : kind === "regex" ? "RE2 regex (e.g. PROJ-\\d+)"
    : "keyword/phrase (e.g. Project Falcon)";

  return (
    <Card style={{ padding: 0, marginTop: 16 }}>
      <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", fontSize: 15, fontWeight: 500 }}>
        Add a guardrail
      </div>
      <div style={{ padding: "16px 22px", display: "flex", gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <select
          value={kind}
          onChange={e => setKind(e.target.value as GuardrailKind)}
          style={{ padding: "7px 10px", borderRadius: "var(--radius-sm)", background: "var(--bg-2)", border: "1px solid var(--border)", color: "inherit", fontFamily: "inherit", fontSize: 13 }}
        >
          <option value="keyword">Keyword</option>
          <option value="regex">Regex</option>
          <option value="category">Category block</option>
        </select>
        <Input size="sm" value={value} onChange={e => setValue(e.target.value)} placeholder={placeholder} style={{ flex: 1, minWidth: 220, fontFamily: "var(--font-mono)" }} onKeyDown={e => { if (e.key === "Enter") create(); }} />
        <Input size="sm" value={label} onChange={e => setLabel(e.target.value)} placeholder="label (optional)" style={{ width: 180 }} />
        <Btn variant="primary" size="sm" onClick={create} disabled={busy || !value.trim()}><I.plus size={14} /> Add</Btn>
      </div>
    </Card>
  );
}

function Thresholds() {
  const toast = useToast();
  const [pol, setPol] = React.useState<OrgPolicy | null>(null);
  const [quality, setQuality] = React.useState("");
  const [nearDup, setNearDup] = React.useState("");
  const [maxPerSync, setMaxPerSync] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  const load = React.useCallback(() => {
    api.getPolicy().then(p => {
      setPol(p);
      setQuality(String(p.quality_threshold));
      setNearDup(String(p.near_dup_threshold));
      setMaxPerSync(String(p.max_memories_per_sync));
    }).catch(() => {});
  }, []);
  React.useEffect(load, [load]);

  if (!pol) return null;

  async function save() {
    setSaving(true);
    try {
      // Categories are managed as guardrails now; pass the policy's existing
      // blocked_categories through untouched and only update the thresholds.
      await api.updatePolicy({
        blocked_categories: pol!.blocked_categories,
        quality_threshold: parseFloat(quality) || 0,
        near_dup_threshold: parseFloat(nearDup) || 0,
        max_memories_per_sync: parseInt(maxPerSync, 10) || 0,
      });
      toast.push({ title: "Thresholds saved", variant: "success" });
    } catch (e) {
      toast.push({ title: "Save failed", description: e instanceof Error ? e.message : "", variant: "error" });
    } finally {
      setSaving(false);
    }
  }

  const lbl = (t: string) => <div style={{ fontSize: 11.5, color: "var(--text-dim)", marginBottom: 6, fontFamily: "var(--font-mono)" }}>{t}</div>;

  return (
    <Card style={{ padding: 0, marginTop: 16 }}>
      <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)" }}>
        <div style={{ fontSize: 15, fontWeight: 500 }}>Scoring thresholds</div>
        <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 4 }}>Quality gate and near-duplicate sensitivity enforced on every sync.</div>
      </div>
      <div style={{ padding: "16px 22px", display: "flex", gap: 14, alignItems: "flex-end", flexWrap: "wrap" }}>
        <div style={{ width: 160 }}>{lbl("Quality threshold (0–1)")}<Input full size="sm" type="number" value={quality} onChange={e => setQuality(e.target.value)} /></div>
        <div style={{ width: 160 }}>{lbl("Near-duplicate (0–1)")}<Input full size="sm" type="number" value={nearDup} onChange={e => setNearDup(e.target.value)} /></div>
        <div style={{ width: 160 }}>{lbl("Max memories / sync")}<Input full size="sm" type="number" value={maxPerSync} onChange={e => setMaxPerSync(e.target.value)} /></div>
        <Btn variant="primary" size="sm" onClick={save} disabled={saving}>{saving ? "Saving…" : "Save thresholds"}</Btn>
      </div>
    </Card>
  );
}

export function GuardrailsPage() {
  const [guards, setGuards] = React.useState<Guardrail[] | null>(null);
  const [err, setErr] = React.useState<string | null>(null);

  const load = React.useCallback(() => {
    api.listGuardrails().then(setGuards).catch(e => setErr(e instanceof Error ? e.message : "failed to load"));
  }, []);
  React.useEffect(load, [load]);

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <I.shield size={20} />
          <h1 style={{ fontSize: 20, fontWeight: 600, margin: 0 }}>Guardrails</h1>
        </div>
        <div style={{ fontSize: 13, color: "var(--text-dim)", marginTop: 6 }}>
          Organization-wide rules enforced on every incoming sync. Comes with useful defaults — disable, adjust, or add your own.
        </div>
      </div>

      {err && <Card style={{ padding: 16, color: "var(--err)" }}>{err}</Card>}

      {guards && (
        <Card style={{ padding: 0 }}>
          <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", fontSize: 15, fontWeight: 500 }}>
            Active guardrails
          </div>
          {guards.length === 0
            ? <div style={{ padding: "16px 22px", color: "var(--text-dim)", fontSize: 13 }}>No guardrails configured — syncs are unfiltered.</div>
            : guards.map(g => <GuardrailRow key={g.id} g={g} onChange={load} />)}
        </Card>
      )}

      <AddGuardrail onCreated={load} />
      <Thresholds />
    </div>
  );
}
