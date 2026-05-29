import React, { useState, useEffect } from "react";
import { Card, ScopeChip, Status, Table, Btn } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useToast } from "@/components/ui/toast";
import type { APIKey, APIKeyMintResponse, Scope } from "@/lib/types";

const SEEN_KEY = "anchored_apikeys_seen";

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

function keyStatus(k: APIKey): { value: string; label: string } {
  if (k.revoked_at) return { value: "offline", label: "revoked" };
  if (k.expires_at && new Date(k.expires_at).getTime() < Date.now()) return { value: "dim", label: "expired" };
  return { value: "online", label: "active" };
}

const EXPIRY_OPTIONS = [
  { label: "1 day", value: "24h" },
  { label: "7 days", value: "168h" },
  { label: "30 days", value: "720h" },
  { label: "90 days", value: "2160h" },
  { label: "Never", value: "" },
];

// ── New key modal ───────────────────────────────────────────────────────────
interface NewKeyModalProps {
  accountId: string;
  onClose: () => void;
  onCreated: (key: APIKey, fullKey: string) => void;
}

function NewKeyModal({ accountId, onClose, onCreated }: NewKeyModalProps) {
  const toast = useToast();
  const [name, setName] = useState("");
  const [scope, setScope] = useState<Scope>("sync");
  const [expiry, setExpiry] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    try {
      const res: APIKeyMintResponse = await api.createAPIKey(name.trim(), scope, accountId, expiry || undefined);
      const stub: APIKey = {
        id: res.id,
        org_id: "",
        account_id: accountId,
        name: res.name,
        key_prefix: res.key.slice(0, 8),
        scope: res.scope,
        expires_at: res.expires_at ?? null,
        created_at: res.created_at,
        revoked_at: null,
      };
      onCreated(stub, res.key);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create key";
      toast.push({ title: "Create failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Backdrop onClose={onClose}>
      <ModalBox>
        <ModalHeader title="Create API key" onClose={onClose} />
        <form onSubmit={submit}>
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div>
              <FieldLabel>Name</FieldLabel>
              <input
                style={inputStyle}
                placeholder="my-agent-key"
                value={name}
                onChange={e => setName(e.target.value)}
                required
                autoFocus
              />
            </div>
            <div>
              <FieldLabel>Scope</FieldLabel>
              <select value={scope} onChange={e => setScope(e.target.value as Scope)} style={selectStyle}>
                <option value="admin">admin</option>
                <option value="sync">sync</option>
                <option value="readonly">readonly</option>
              </select>
            </div>
            <div>
              <FieldLabel>Expiry</FieldLabel>
              <select value={expiry} onChange={e => setExpiry(e.target.value)} style={selectStyle}>
                {EXPIRY_OPTIONS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>
            <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
              <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
              <Btn variant="primary" size="md" full>
                {submitting ? "Creating…" : "Create key"}
              </Btn>
            </div>
          </div>
        </form>
      </ModalBox>
    </Backdrop>
  );
}

// ── Show full key modal (one-time) ──────────────────────────────────────────
interface ShowKeyModalProps {
  fullKey: string;
  onClose: () => void;
}

function ShowKeyModal({ fullKey, onClose }: ShowKeyModalProps) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try { await navigator.clipboard.writeText(fullKey); } catch { /* ignore */ }
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  }

  return (
    <Backdrop onClose={onClose}>
      <ModalBox>
        <ModalHeader title="Save your API key" onClose={onClose} />
        <div style={{
          padding: "10px 12px", marginBottom: 16,
          background: "var(--warn-bg)", border: "1px solid color-mix(in srgb, var(--warn) 25%, transparent)",
          borderRadius: "var(--radius)", fontSize: 13, color: "var(--warn)", lineHeight: 1.5,
        }}>
          This is the only time the full key is shown. Copy it now.
        </div>
        <div style={{
          display: "flex", alignItems: "center", gap: 8,
          background: "var(--bg-1)", border: "1px solid var(--border)",
          borderRadius: "var(--radius)", padding: "10px 12px", marginBottom: 20,
        }}>
          <code style={{ flex: 1, fontFamily: "var(--font-mono)", fontSize: 12.5, wordBreak: "break-all" as const, color: "var(--text)" }}>
            {fullKey}
          </code>
          <button type="button" onClick={copy} style={{
            border: 0, background: "transparent", color: copied ? "var(--ok)" : "var(--text-dim)",
            cursor: "pointer", display: "inline-flex", alignItems: "center", gap: 4,
            fontFamily: "var(--font-mono)", fontSize: 11, padding: "2px 6px",
          }}>
            {copied ? <I.check size={13} /> : <I.copy size={13} />}
            {copied ? "copied" : "copy"}
          </button>
        </div>
        <Btn variant="primary" size="md" full onClick={onClose}>Done — I've saved it</Btn>
      </ModalBox>
    </Backdrop>
  );
}

// ── Shared modal primitives ─────────────────────────────────────────────────
function Backdrop({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  return (
    <div onClick={onClose} style={{
      position: "fixed", inset: 0, zIndex: 50,
      background: "rgba(0,0,0,0.6)",
      display: "flex", alignItems: "center", justifyContent: "center",
    }}>
      <div onClick={e => e.stopPropagation()}>{children}</div>
    </div>
  );
}

function ModalBox({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      background: "var(--bg-2)", border: "1px solid var(--border)",
      borderRadius: "var(--radius-lg)", padding: 28, width: 420,
      boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
    }}>
      {children}
    </div>
  );
}

function ModalHeader({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
      <div style={{ fontSize: 16, fontWeight: 500 }}>{title}</div>
      <button type="button" onClick={onClose} style={{
        border: 0, background: "transparent", color: "var(--text-dim)",
        cursor: "pointer", display: "inline-flex", padding: 4,
      }}><I.x size={16} /></button>
    </div>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
      {children}
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
  background: "var(--bg-input)", border: "1px solid var(--border)",
  borderRadius: "var(--radius)", color: "var(--text)", boxSizing: "border-box",
};

const selectStyle: React.CSSProperties = {
  width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
  background: "var(--bg-input)", border: "1px solid var(--border)",
  borderRadius: "var(--radius)", color: "var(--text)", cursor: "pointer",
};

// ── Main page ───────────────────────────────────────────────────────────────
export function APIKeysPage() {
  const { me } = useAuth();
  const toast = useToast();
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [showNewModal, setShowNewModal] = useState(false);
  const [fullKey, setFullKey] = useState<string | null>(null);
  const [showWelcome, setShowWelcome] = useState(() => !localStorage.getItem(SEEN_KEY));

  useEffect(() => {
    api.getAPIKeys()
      .then(setKeys)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  function dismissWelcome() {
    localStorage.setItem(SEEN_KEY, "1");
    setShowWelcome(false);
  }

  function handleKeyCreated(key: APIKey, kFull: string) {
    setKeys(prev => [key, ...prev]);
    setShowNewModal(false);
    setFullKey(kFull);
  }

  async function revokeKey(id: string) {
    if (!window.confirm("Revoke this API key? Any client or agent still using it will immediately stop syncing. This cannot be undone.")) return;
    try {
      await api.revokeAPIKey(id);
      setKeys(prev => prev.map(k => k.id === id ? { ...k, revoked_at: new Date().toISOString() } : k));
      toast.push({ title: "Key revoked", variant: "success" });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to revoke";
      toast.push({ title: "Revoke failed", description: msg, variant: "error" });
    }
  }

  async function rotateKey(k: APIKey) {
    if (!window.confirm(`Rotate "${k.name}"? The current key is revoked immediately and replaced with a new one — update any client using it.`)) return;
    try {
      await api.revokeAPIKey(k.id);
      const res: APIKeyMintResponse = await api.createAPIKey(k.name, k.scope, k.account_id, undefined);
      const stub: APIKey = {
        id: res.id,
        org_id: k.org_id,
        account_id: k.account_id,
        name: res.name,
        key_prefix: res.key.slice(0, 8),
        scope: res.scope,
        expires_at: res.expires_at ?? null,
        created_at: res.created_at,
        revoked_at: null,
      };
      setKeys(prev => [stub, ...prev.map(x => x.id === k.id ? { ...x, revoked_at: new Date().toISOString() } : x)]);
      setFullKey(res.key);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Rotation failed";
      toast.push({ title: "Rotate failed", description: msg, variant: "error" });
    }
  }

  // Non-admins only see their own keys
  const visibleKeys = me?.scope === "admin"
    ? keys
    : keys.filter(k => k.account_id === me?.account_id);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
      {/* Welcome banner */}
      {showWelcome && (
        <Card style={{ padding: 18, marginBottom: 18, border: "1px solid var(--accent-border)" }}>
          <div style={{ display: "flex", alignItems: "flex-start", gap: 14 }}>
            <div style={{
              width: 32, height: 32, borderRadius: 6,
              background: "var(--accent-bg)", color: "var(--accent)",
              display: "inline-flex", alignItems: "center", justifyContent: "center",
              border: "1px solid var(--accent-border)", flex: "none",
            }}>
              <I.key size={16} />
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 500, marginBottom: 4 }}>Connect your CLI</div>
              <div style={{ fontSize: 13, color: "var(--text-muted)", lineHeight: 1.55, maxWidth: 600 }}>
                Create an API key below, then run{" "}
                <code style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>
                  anchored remote login --server {window.location.origin} --key &lt;key&gt;
                </code>{" "}
                to connect.
              </div>
            </div>
            <button type="button" onClick={dismissWelcome} style={{
              border: 0, background: "transparent", color: "var(--text-dim)",
              cursor: "pointer", display: "inline-flex", padding: 4,
            }}><I.x size={14} /></button>
          </div>
        </Card>
      )}

      {/* Scopes info card */}
      <Card style={{ padding: 18, marginBottom: 18, background: "var(--bg-1)", border: "1px solid var(--accent-border)" }}>
        <div style={{ display: "flex", alignItems: "flex-start", gap: 14 }}>
          <div style={{
            width: 32, height: 32, borderRadius: 6,
            background: "var(--accent-bg)", color: "var(--accent)",
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            border: "1px solid var(--accent-border)", flex: "none",
          }}>
            <I.key size={16} />
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 14, fontWeight: 500 }}>Scopes</div>
            <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4, lineHeight: 1.55, maxWidth: 720 }}>
              Anchored OSS keys carry three scope levels.{" "}
              <code style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>admin</code> manages org/teams/policies,{" "}
              <code style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>sync</code> reads & writes memories within a project, and{" "}
              <code style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>readonly</code> can fetch memories for an agent but cannot append.
            </div>
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <ScopeChip scope="admin" />
            <ScopeChip scope="sync" />
            <ScopeChip scope="readonly" />
          </div>
        </div>
      </Card>

      {/* Actions bar */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "flex-end", marginBottom: 14 }}>
        <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowNewModal(true)}>
          Create key
        </Btn>
      </div>

      {visibleKeys.length === 0 ? (
        <Card style={{ padding: "40px 22px", textAlign: "center" }}>
          <div style={{ fontSize: 13, color: "var(--text-dim)" }}>No API keys yet.</div>
        </Card>
      ) : (
        <Card>
          <Table
            cols={[
              { key: "name", label: "Name" },
              { key: "preview", label: "Key", mono: true },
              { key: "scope", label: "Scope" },
              { key: "created", label: "Created", mono: true, muted: true },
              { key: "expires", label: "Expires", mono: true, muted: true },
              { key: "status", label: "Status" },
              { key: "actions", label: "", align: "right" as const },
            ]}
            rows={visibleKeys.map(k => {
              const st = keyStatus(k);
              const active = !k.revoked_at && !(k.expires_at && new Date(k.expires_at).getTime() < Date.now());
              return {
                name: k.name,
                preview: k.key_prefix + " ····",
                scope: <ScopeChip scope={k.scope} />,
                created: timeAgo(k.created_at),
                expires: k.expires_at ? new Date(k.expires_at).toLocaleDateString() : "never",
                status: <Status value={st.value} label={st.label} />,
                actions: (
                  <div style={{ display: "flex", gap: 4, justifyContent: "flex-end" }}>
                    {active && (
                      <Btn variant="ghost" size="sm" icon={<I.refresh />} onClick={() => rotateKey(k)} />
                    )}
                    {active && (
                      <Btn variant="ghost" size="sm" icon={<I.trash />} onClick={() => revokeKey(k.id)}
                        style={{ color: "var(--err)" }} />
                    )}
                  </div>
                ),
              };
            })}
          />
        </Card>
      )}

      <Card style={{ padding: 18, marginTop: 18 }}>
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase", marginBottom: 12 }}>
          Using a key
        </div>
        <div style={{
          background: "var(--bg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius)",
          fontFamily: "var(--font-mono)", fontSize: 12.5, lineHeight: 1.7, padding: "12px 14px",
          color: "var(--text)",
        }}>
          <div>$ export ANCHORED_OSS_URL="{window.location.origin}"</div>
          <div>$ export ANCHORED_OSS_KEY="ak_..."</div>
          <div>$ anchored sync --to my-project</div>
        </div>
      </Card>

      {showNewModal && me && (
        <NewKeyModal
          accountId={me.account_id}
          onClose={() => setShowNewModal(false)}
          onCreated={handleKeyCreated}
        />
      )}

      {fullKey && (
        <ShowKeyModal fullKey={fullKey} onClose={() => setFullKey(null)} />
      )}
    </div>
  );
}
