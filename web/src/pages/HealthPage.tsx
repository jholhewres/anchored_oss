import React from "react";
import { Card, Status } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { Health } from "@/lib/types";

function UptimeBar() {
  return (
    <div style={{ display: "flex", gap: 2, height: 22 }}>
      {Array.from({ length: 60 }).map((_, i) => (
        <div key={i} style={{ flex: 1, background: "var(--ok)", borderRadius: 1, opacity: 0.9 }} />
      ))}
    </div>
  );
}

function KV({ k, v }: { k: string; v: string | React.ReactNode }) {
  return (
    <div>
      <div style={{ fontSize: 10.5, fontFamily: "var(--font-mono)", color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>{k}</div>
      <div style={{ fontFamily: "var(--font-mono)", fontSize: 13.5, color: "var(--text)", marginTop: 4, fontWeight: 500 }}>{v}</div>
    </div>
  );
}

export function HealthPage() {
  const [health, setHealth] = React.useState<Health | null>(null);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    api.getHealth()
      .then(setHealth)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  const isOk = health?.status === "ok" && health?.db_status === "ok";

  return (
    <div>
      <Card style={{
        padding: 22, marginBottom: 14, display: "flex", alignItems: "center", justifyContent: "space-between",
        background: isOk ? "var(--ok-bg)" : "var(--err-bg)",
        border: isOk ? "1px solid color-mix(in srgb, var(--ok) 25%, transparent)" : "1px solid color-mix(in srgb, var(--err) 25%, transparent)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
          <span style={{
            width: 36, height: 36, borderRadius: 8,
            background: isOk ? "color-mix(in srgb, var(--ok) 15%, transparent)" : "color-mix(in srgb, var(--err) 15%, transparent)",
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            color: isOk ? "var(--ok)" : "var(--err)",
          }}>
            <I.check size={20} />
          </span>
          <div>
            <div style={{ fontSize: 18, fontWeight: 500 }}>
              {isOk ? "All systems operational" : "Service degraded"}
            </div>
            <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 4 }}>
              {health ? `Service: ${health.service} · v${health.version}` : "No health data"}
            </div>
          </div>
        </div>
        {health && (
          <div style={{ display: "flex", gap: 24, fontFamily: "var(--font-mono)" }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>service</div>
              <div style={{ fontSize: 22, color: isOk ? "var(--ok)" : "var(--err)", fontWeight: 500 }}>{health.status}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>database</div>
              <div style={{ fontSize: 22, color: health.db_status === "ok" ? "var(--ok)" : "var(--err)", fontWeight: 500 }}>{health.db_status}</div>
            </div>
          </div>
        )}
      </Card>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 14 }}>
        <Card style={{ padding: 18 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 14, fontWeight: 500 }}>service</div>
              <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 3 }}>
                {health ? `${health.service} · v${health.version}` : "—"}
              </div>
            </div>
            <Status value={health?.status === "ok" ? "online" : "offline"} label={health?.status || "unknown"} />
          </div>
          <UptimeBar />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10, marginTop: 14, paddingTop: 14, borderTop: "1px solid var(--border)" }}>
            <KV k="status" v={health?.status || "—"} />
            <KV k="version" v={health?.version || "—"} />
            <KV k="checked" v={health ? new Date(health.timestamp).toLocaleTimeString() : "—"} />
          </div>
        </Card>

        <Card style={{ padding: 18 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 14, fontWeight: 500 }}>database</div>
              <div style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 3 }}>PostgreSQL connection</div>
            </div>
            <Status value={health?.db_status === "ok" ? "online" : "offline"} label={health?.db_status || "unknown"} />
          </div>
          <UptimeBar />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10, marginTop: 14, paddingTop: 14, borderTop: "1px solid var(--border)" }}>
            <KV k="db status" v={health?.db_status || "—"} />
            <KV k="service" v={health?.service || "—"} />
            <KV k="timestamp" v={health ? new Date(health.timestamp).toLocaleTimeString() : "—"} />
          </div>
        </Card>
      </div>
    </div>
  );
}
