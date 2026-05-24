import React from "react";
import { Card, ScopeChip, Status, Table } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { APIKey } from "@/lib/types";

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

export function APIKeysPage() {
  const [keys, setKeys] = React.useState<APIKey[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    api.getAPIKeys()
      .then(setKeys)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
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

      {keys.length === 0 ? (
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
              { key: "status", label: "Status", align: "right" as const },
            ]}
            rows={keys.map(k => {
              const st = keyStatus(k);
              return {
                name: k.name,
                preview: k.key_prefix + " ····",
                scope: <ScopeChip scope={k.scope} />,
                created: timeAgo(k.created_at),
                expires: k.expires_at ? new Date(k.expires_at).toLocaleDateString() : "never",
                status: <Status value={st.value} label={st.label} />,
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
          <div>$ export ANCHORED_OSS_URL="http://localhost:8080"</div>
          <div>$ export ANCHORED_OSS_KEY="ak_..."</div>
          <div>$ anchored sync --to my-project</div>
        </div>
      </Card>
    </div>
  );
}
