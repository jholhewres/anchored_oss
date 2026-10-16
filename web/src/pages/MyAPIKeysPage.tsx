import { useCallback, useEffect, useState, type CSSProperties, type FormEvent, type ReactNode } from "react";

import { useToast } from "@/components/ui/toast";
import { Card, ClipboardButton, Empty, ScopeChip, Status, Table, Btn } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { APIKey } from "@/lib/types";

type PersonalKeyScope = "sync" | "readonly";
type PersonalKeyExpiry = "30d" | "90d" | "180d" | "365d";

const EXPIRY_OPTIONS: { label: string; value: PersonalKeyExpiry }[] = [
  { label: "30 days", value: "30d" },
  { label: "90 days (recommended)", value: "90d" },
  { label: "180 days", value: "180d" },
  { label: "365 days", value: "365d" },
];

function timeAgo(dateStr: string): string {
  const elapsed = Date.now() - new Date(dateStr).getTime();
  const seconds = Math.floor(elapsed / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function keyStatus(key: APIKey): { value: string; label: string } {
  if (key.revoked_at) return { value: "offline", label: "revoked" };
  if (key.expires_at && new Date(key.expires_at).getTime() < Date.now()) {
    return { value: "dim", label: "expired" };
  }
  return { value: "online", label: "active" };
}

function isActive(key: APIKey): boolean {
  return !key.revoked_at && (!key.expires_at || new Date(key.expires_at).getTime() >= Date.now());
}

function Backdrop({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  return (
    <div
      role="presentation"
      onClick={onClose}
      style={{
        position: "fixed", inset: 0, zIndex: 50, background: "rgba(0,0,0,0.6)",
        display: "flex", alignItems: "center", justifyContent: "center", padding: 16,
      }}
    >
      <div role="dialog" aria-modal="true" aria-labelledby="api-key-modal-title" onClick={event => event.stopPropagation()}>{children}</div>
    </div>
  );
}

function ModalBox({ children }: { children: ReactNode }) {
  return (
    <div style={{
      background: "var(--bg-2)", border: "1px solid var(--border)", borderRadius: "var(--radius-lg)",
      padding: 28, width: "min(420px, calc(100vw - 32px))", boxSizing: "border-box",
      boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
    }}>
      {children}
    </div>
  );
}

function ModalHeader({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
      <div id="api-key-modal-title" style={{ fontSize: 16, fontWeight: 500 }}>{title}</div>
      <button
        type="button"
        aria-label="Close dialog"
        onClick={onClose}
        style={closeButtonStyle}
      >
        <I.x size={16} />
      </button>
    </div>
  );
}

function FieldLabel({ children }: { children: ReactNode }) {
  return <div style={fieldLabelStyle}>{children}</div>;
}

interface CreateKeyModalProps {
  canCreateSyncKey: boolean;
  onClose: () => void;
  onCreated: (key: APIKey, secret: string) => void;
}

function CreateKeyModal({ canCreateSyncKey, onClose, onCreated }: CreateKeyModalProps) {
  const toast = useToast();
  const [name, setName] = useState("");
  const [scope, setScope] = useState<PersonalKeyScope>(canCreateSyncKey ? "sync" : "readonly");
  const [expiry, setExpiry] = useState<PersonalKeyExpiry>("90d");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || submitting) return;

    setSubmitting(true);
    try {
      const response = await api.createMyAPIKey(trimmedName, canCreateSyncKey ? scope : "readonly", expiry);
      onCreated({
        id: response.id,
        org_id: "",
        account_id: "",
        name: response.name,
        key_prefix: response.key.slice(0, 8),
        scope: response.scope,
        expires_at: response.expires_at ?? null,
        created_at: response.created_at,
        revoked_at: null,
      }, response.key);
    } catch (error) {
      toast.push({
        title: "Could not create API key",
        description: error instanceof Error ? error.message : "Please try again.",
        variant: "error",
      });
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
                autoFocus
                required
                maxLength={120}
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder="my-agent-key"
                style={inputStyle}
              />
            </div>
            <div>
              <FieldLabel>Access</FieldLabel>
              <select value={canCreateSyncKey ? scope : "readonly"} onChange={event => setScope(event.target.value as PersonalKeyScope)} style={inputStyle}>
                {canCreateSyncKey && <option value="sync">sync — read and write memories</option>}
                <option value="readonly">readonly — fetch memories only</option>
              </select>
              {!canCreateSyncKey && <div style={{ color: "var(--text-dim)", fontSize: 12, marginTop: 6 }}>Your readonly session can create readonly keys only.</div>}
            </div>
            <div>
              <FieldLabel>Expiry</FieldLabel>
              <select value={expiry} onChange={event => setExpiry(event.target.value as PersonalKeyExpiry)} style={inputStyle}>
                {EXPIRY_OPTIONS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              <div style={{ color: "var(--text-dim)", fontSize: 12, marginTop: 6 }}>
                Personal keys expire after 90 days by default and cannot exceed one year.
              </div>
            </div>
            <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
              <Btn type="button" variant="outline" size="md" disabled={submitting} onClick={onClose}>Cancel</Btn>
              <Btn type="submit" variant="primary" size="md" full disabled={!name.trim() || submitting}>
                {submitting ? "Creating…" : "Create key"}
              </Btn>
            </div>
          </div>
        </form>
      </ModalBox>
    </Backdrop>
  );
}

function SecretModal({ secret, onClose }: { secret: string; onClose: () => void }) {
  const toast = useToast();

  return (
    <Backdrop onClose={onClose}>
      <ModalBox>
        <ModalHeader title="Save your API key" onClose={onClose} />
        <div style={{
          padding: "10px 12px", marginBottom: 16, background: "var(--warn-bg)",
          border: "1px solid color-mix(in srgb, var(--warn) 25%, transparent)", borderRadius: "var(--radius)",
          fontSize: 13, color: "var(--warn)", lineHeight: 1.5,
        }}>
          This is the only time the full key will be displayed. Copy it and store it somewhere safe.
        </div>
        <div style={{
          display: "flex", alignItems: "center", gap: 8, padding: "10px 12px", marginBottom: 20,
          background: "var(--bg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius)",
        }}>
          <code style={{ flex: 1, minWidth: 0, fontFamily: "var(--font-mono)", fontSize: 12.5, wordBreak: "break-all", color: "var(--text)" }}>
            {secret}
          </code>
          <ClipboardButton
            text={secret}
            label="Copy"
            successLabel="Copied"
            errorLabel="Copy failed"
            ariaLabel="Copy API key"
            onCopied={success => toast.push({
              title: success ? "API key copied" : "Copy failed",
              description: success ? undefined : "Select the key and copy it manually.",
              variant: success ? "success" : "error",
            })}
          />
        </div>
        <Btn variant="primary" size="md" full onClick={onClose}>Done — I saved it</Btn>
      </ModalBox>
    </Backdrop>
  );
}

export function MyAPIKeysPage() {
  const toast = useToast();
  const { me } = useAuth();
  const canCreateSyncKey = me?.scope === "sync" || me?.scope === "admin";
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [revokingID, setRevokingID] = useState<string | null>(null);

  const loadKeys = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setKeys(await api.getMyAPIKeys());
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "Unable to load your API keys.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadKeys();
  }, [loadKeys]);

  function handleCreated(key: APIKey, fullKey: string) {
    setKeys(previous => [key, ...previous]);
    setShowCreate(false);
    setSecret(fullKey);
  }

  async function revokeKey(key: APIKey) {
    if (!window.confirm(`Revoke "${key.name}"? Any client using this key will immediately lose access. This cannot be undone.`)) {
      return;
    }

    setRevokingID(key.id);
    try {
      await api.revokeMyAPIKey(key.id);
      setKeys(previous => previous.map(item => item.id === key.id ? { ...item, revoked_at: new Date().toISOString() } : item));
      toast.push({ title: "API key revoked", variant: "success" });
    } catch (revokeError) {
      toast.push({
        title: "Could not revoke API key",
        description: revokeError instanceof Error ? revokeError.message : "Please try again.",
        variant: "error",
      });
    } finally {
      setRevokingID(null);
    }
  }

  if (loading) {
    return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading your API keys…</div>;
  }

  if (error) {
    return (
      <Card style={{ padding: 24 }}>
        <div style={{ fontSize: 14, fontWeight: 500, marginBottom: 6 }}>Could not load API keys</div>
        <div style={{ color: "var(--text-muted)", fontSize: 13, marginBottom: 16 }}>{error}</div>
        <Btn variant="outline" size="sm" onClick={() => void loadKeys()}>Try again</Btn>
      </Card>
    );
  }

  return (
    <div>
      <div className="my-api-keys-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16, marginBottom: 18 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>My API keys</h1>
          <div style={{ color: "var(--text-muted)", fontSize: 13, marginTop: 5 }}>
            Create credentials for your own clients and revoke them when they are no longer needed.
          </div>
        </div>
        <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowCreate(true)}>Create key</Btn>
      </div>

      <Card style={{ padding: 18, marginBottom: 18, background: "var(--bg-1)", border: "1px solid var(--accent-border)" }}>
        <div style={{ display: "flex", alignItems: "flex-start", gap: 12 }}>
          <div style={{ color: "var(--accent)", paddingTop: 2 }}><I.key size={16} /></div>
          <div style={{ fontSize: 13, color: "var(--text-muted)", lineHeight: 1.55 }}>
            Personal keys can use <ScopeChip scope="sync" /> to read and write memories, or <ScopeChip scope="readonly" /> to fetch them only. They default to 90 days and expire within one year.{me?.scope === "readonly" && " Your readonly session can create readonly keys only."}
          </div>
        </div>
      </Card>

      {keys.length === 0 ? (
        <Empty
          icon={<I.key />}
          title="No personal API keys yet"
          body="Create a key to connect your local client or agent. The full key is shown only once."
          actions={<Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowCreate(true)}>Create key</Btn>}
        />
      ) : (
        <Card>
          <Table
            cols={[
              { key: "name", label: "Name" },
              { key: "preview", label: "Key", mono: true },
              { key: "scope", label: "Access" },
              { key: "created", label: "Created", mono: true, muted: true },
              { key: "expires", label: "Expires", mono: true, muted: true },
              { key: "status", label: "Status" },
              { key: "actions", label: "", align: "right" as const },
            ]}
            rows={keys.map(key => {
              const status = keyStatus(key);
              return {
                name: key.name,
                preview: `${key.key_prefix} ····`,
                scope: <ScopeChip scope={key.scope} />,
                created: timeAgo(key.created_at),
                expires: key.expires_at ? new Date(key.expires_at).toLocaleDateString() : "—",
                status: <Status value={status.value} label={status.label} />,
                actions: isActive(key) ? (
                  <Btn
                    variant="ghost"
                    size="sm"
                    icon={<I.trash />}
                    aria-label={`Revoke ${key.name}`}
                    title={`Revoke ${key.name}`}
                    disabled={revokingID === key.id}
                    onClick={() => void revokeKey(key)}
                    style={{ color: "var(--err)" }}
                  >
                    {revokingID === key.id ? "Revoking…" : "Revoke"}
                  </Btn>
                ) : null,
              };
            })}
          />
        </Card>
      )}

      <Card style={{ padding: 18, marginTop: 18 }}>
        <div style={fieldLabelStyle}>Use a key</div>
        <div className="api-key-command" style={{
          background: "var(--bg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius)",
          fontFamily: "var(--font-mono)", fontSize: 12.5, lineHeight: 1.7, padding: "12px 14px", color: "var(--text)", overflowX: "auto",
        }}>
          <div>$ export ANCHORED_OSS_URL="{window.location.origin}"</div>
          <div>$ export ANCHORED_OSS_KEY="ak_..."</div>
          <div>$ anchored sync --to my-project</div>
        </div>
      </Card>

      {showCreate && <CreateKeyModal canCreateSyncKey={canCreateSyncKey} onClose={() => setShowCreate(false)} onCreated={handleCreated} />}
      {secret && <SecretModal secret={secret} onClose={() => setSecret(null)} />}
    </div>
  );
}

const inputStyle: CSSProperties = {
  width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
  background: "var(--bg-input)", border: "1px solid var(--border)", borderRadius: "var(--radius)",
  color: "var(--text)", boxSizing: "border-box",
};

const fieldLabelStyle: CSSProperties = {
  fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4,
  textTransform: "uppercase", marginBottom: 6,
};

const closeButtonStyle: CSSProperties = {
  border: 0, background: "transparent", color: "var(--text-dim)", cursor: "pointer", display: "inline-flex", padding: 4,
};
