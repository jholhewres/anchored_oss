import React from "react";
import { Card, Btn, Input } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { AuditEntry, AuditFilters } from "@/lib/types";

const actionTones: Record<string, string> = {
  "sync.push.accepted": "ok",
  "sync.push.rejected": "err",
  "sync.tombstone.accepted": "warn",
  "sync.project.created": "info",
  "memory.append": "accent",
  "apikey.rotate": "warn",
  "member.invite": "info",
  "policy.update": "neutral",
};

const selectStyle: React.CSSProperties = {
  height: 28, padding: "0 8px", fontSize: 12,
  background: "var(--bg-2)", color: "var(--text)",
  border: "1px solid var(--border)", borderRadius: "var(--radius)",
  fontFamily: "var(--font-mono)", cursor: "pointer",
};

// rangeFrom returns an ISO timestamp for the start of the selected window, or
// undefined for "all". Used to populate the audit `from` filter.
function rangeFrom(key: string): string | undefined {
  const now = Date.now();
  const day = 24 * 60 * 60 * 1000;
  switch (key) {
    case "24h": return new Date(now - day).toISOString();
    case "7d": return new Date(now - 7 * day).toISOString();
    case "30d": return new Date(now - 30 * day).toISOString();
    default: return undefined;
  }
}

export function AuditPage() {
  const [entries, setEntries] = React.useState<AuditEntry[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [filters, setFilters] = React.useState<AuditFilters>({ limit: 50, offset: 0 });
  const [search, setSearch] = React.useState("");
  const [rangeKey, setRangeKey] = React.useState("all");

  React.useEffect(() => {
    setLoading(true);
    api.getAudit(filters)
      .then(r => { setEntries(r.entries); setTotal(r.total); })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [filters]);

  if (loading && entries.length === 0) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  const offset = filters.offset ?? 0;

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16, flexWrap: "wrap" }}>
        <Input
          icon={<I.user />}
          placeholder="actor id… (Enter)"
          size="sm"
          style={{ width: 220 }}
          value={search}
          onChange={e => setSearch(e.target.value)}
          onKeyDown={e => { if (e.key === "Enter") setFilters(f => ({ ...f, actor: search.trim() || undefined, offset: 0 })); }}
        />
        <select value={filters.action ?? ""} onChange={e => setFilters(f => ({ ...f, action: e.target.value || undefined, offset: 0 }))} style={selectStyle}>
          <option value="">kind: all</option>
          {Object.keys(actionTones).map(a => <option key={a} value={a}>{a}</option>)}
        </select>
        <select value={rangeKey} onChange={e => { setRangeKey(e.target.value); setFilters(f => ({ ...f, from: rangeFrom(e.target.value), offset: 0 })); }} style={selectStyle}>
          <option value="all">time: all</option>
          <option value="24h">last 24h</option>
          <option value="7d">last 7d</option>
          <option value="30d">last 30d</option>
        </select>
        {(filters.action || filters.actor || filters.from) && (
          <Btn variant="ghost" size="sm" onClick={() => { setSearch(""); setRangeKey("all"); setFilters({ limit: 50, offset: 0 }); }}>Clear</Btn>
        )}
        <div style={{ flex: 1 }} />
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>{total} events</span>
      </div>

      <Card>
        <div style={{
          padding: "10px 14px", borderBottom: "1px solid var(--border)",
          background: "var(--bg-1)", fontFamily: "var(--font-mono)", fontSize: 11,
          color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase",
          display: "grid", gridTemplateColumns: "90px 24px 200px 140px 140px 1fr", gap: 14,
        }}>
          <span>time</span>
          <span />
          <span>kind</span>
          <span>actor</span>
          <span>project</span>
          <span>detail</span>
        </div>
        {entries.length === 0 ? (
          <div style={{ padding: "32px 14px", textAlign: "center", color: "var(--text-dim)", fontSize: 13 }}>No audit entries.</div>
        ) : (
          entries.map((e, i) => {
            const tone = actionTones[e.action] || "neutral";
            const toneVar = tone === "neutral" ? "text-ghost" : tone === "accent" ? "accent" : tone;
            const timeStr = new Date(e.created_at).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
            return (
              <div key={e.id} style={{
                padding: "8px 14px", borderBottom: i < entries.length - 1 ? "1px solid var(--border)" : "none",
                display: "grid", gridTemplateColumns: "90px 24px 200px 140px 140px 1fr",
                alignItems: "center", gap: 14, fontFamily: "var(--font-mono)", fontSize: 12.5,
              }}>
                <span style={{ color: "var(--text-dim)" }}>{timeStr}</span>
                <span style={{
                  width: 6, height: 6, borderRadius: 3,
                  background: `var(--${toneVar})`, justifySelf: "center",
                }} />
                <span style={{ color: "var(--text)" }}>{e.action}</span>
                <span style={{ color: "var(--text-muted)" }}>{e.actor_id ? e.actor_id.slice(0, 8) : "system"}</span>
                <span style={{ color: "var(--text-muted)" }}>{e.project_id ? e.project_id.slice(0, 8) : "—"}</span>
                <span style={{ color: "var(--text)" }}>
                  {e.target_type ? `${e.target_type}/${e.target_id?.slice(0, 8)}` : ""}
                </span>
              </div>
            );
          })
        )}
      </Card>

      <div style={{ marginTop: 14, display: "flex", justifyContent: "flex-end", gap: 8 }}>
        <Btn variant="ghost" size="sm" style={offset === 0 ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setFilters(f => ({ ...f, offset: Math.max(0, offset - 50) }))}>
          Previous
        </Btn>
        <Btn variant="ghost" size="sm" style={offset + 50 >= total ? { opacity: 0.4, pointerEvents: "none" } : {}} onClick={() => setFilters(f => ({ ...f, offset: offset + 50 }))}>
          Next
        </Btn>
      </div>
    </div>
  );
}
