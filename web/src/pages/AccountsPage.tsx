import React from "react";
import { Card, ScopeChip, Status, Btn, Input, Avatar, Table } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { AccountWithRole } from "@/lib/types";

function AccCell({ name, sub }: { name: string; sub?: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
      <Avatar name={name} size={28} />
      <div>
        <div style={{ fontSize: 13.5, fontWeight: 500 }}>{name}</div>
        {sub && <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>{sub}</div>}
      </div>
    </div>
  );
}

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

export function AccountsPage() {
  const [accounts, setAccounts] = React.useState<AccountWithRole[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [search, setSearch] = React.useState("");

  React.useEffect(() => {
    api.getAccounts()
      .then(setAccounts)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const filtered = search
    ? accounts.filter(a => a.email.toLowerCase().includes(search.toLowerCase()) || a.display_name.toLowerCase().includes(search.toLowerCase()))
    : accounts;

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16 }}>
        <Input icon={<I.search />} placeholder="search accounts..." size="sm" style={{ width: 320 }} value={search} onChange={e => setSearch(e.target.value)} />
        <Btn variant="outline" size="sm" icon={<I.filter />}>role: any</Btn>
        <div style={{ flex: 1 }} />
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
          {filtered.length} accounts · {filtered.filter(a => a.role === "admin").length} admins
        </span>
      </div>

      {filtered.length === 0 ? (
        <Card style={{ padding: "40px 22px", textAlign: "center" }}>
          <div style={{ fontSize: 13, color: "var(--text-dim)" }}>No accounts found.</div>
        </Card>
      ) : (
        <Card>
          <Table
            cols={[
              { key: "name", label: "Account" },
              { key: "email", label: "Email", mono: true, muted: true },
              { key: "role", label: "Role" },
              { key: "created", label: "Created", mono: true, muted: true },
              { key: "status", label: "Status", align: "right" as const },
            ]}
            rows={filtered.map(a => ({
              name: <AccCell name={a.display_name} />,
              email: a.email,
              role: <ScopeChip scope={a.role} />,
              created: timeAgo(a.created_at),
              status: <Status value="online" label="active" />,
            }))}
          />
        </Card>
      )}
    </div>
  );
}
