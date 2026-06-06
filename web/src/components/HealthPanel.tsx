import React from "react";
import { Card, Badge } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import type { MemoryHealth } from "@/lib/types";

function scoreTone(score: number): { color: string; label: string } {
  if (score >= 0.85) return { color: "var(--ok, #3fb950)", label: "healthy" };
  if (score >= 0.6) return { color: "var(--warn, #d29922)", label: "needs attention" };
  return { color: "var(--err, #f85149)", label: "unhealthy" };
}

function CountCard({ label, value, alert }: { label: string; value: number; alert?: boolean }) {
  return (
    <Card style={{ padding: "14px 16px", flex: "1 1 120px", minWidth: 120 }}>
      <div style={{ fontSize: 11, color: "var(--text-dim)", textTransform: "uppercase", letterSpacing: 0.4 }}>
        {label}
      </div>
      <div style={{
        fontSize: 22, fontFamily: "var(--font-mono)", marginTop: 6,
        color: alert && value > 0 ? "var(--warn, #d29922)" : "var(--text)",
      }}>
        {value.toLocaleString()}
      </div>
    </Card>
  );
}

// HealthPanel renders the anti context-poisoning view of a project's memory:
// quality score, lifecycle counters, noisiest sources (with volume-spike
// badges), age spread, sync rejection pressure and recommended actions.
export function HealthPanel({ projectId }: { projectId: string }) {
  const [health, setHealth] = React.useState<MemoryHealth | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    api.getProjectMemoryHealth(projectId)
      .then(h => { if (!cancelled) setHealth(h); })
      .catch(e => { if (!cancelled) setError(e?.message ?? "failed to load memory health"); });
    return () => { cancelled = true; };
  }, [projectId]);

  if (error) {
    return (
      <Card style={{ padding: "30px 22px", textAlign: "center", marginTop: 22 }}>
        <div style={{ fontSize: 13, color: "var(--err)" }}>{error}</div>
      </Card>
    );
  }
  if (!health) {
    return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading health...</div>;
  }

  const tone = scoreTone(health.score);
  const spikySources = new Set(health.anomalies.map(a => a.source));

  return (
    <div style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 16 }}>
      {/* Score header */}
      <Card style={{ padding: "18px 20px", display: "flex", alignItems: "center", gap: 18 }}>
        <div style={{
          width: 52, height: 52, borderRadius: "50%", border: `2px solid ${tone.color}`,
          display: "inline-flex", alignItems: "center", justifyContent: "center",
          fontFamily: "var(--font-mono)", fontSize: 15, color: tone.color, flex: "none",
        }}>
          {Math.round(health.score * 100)}
        </div>
        <div>
          <div style={{ fontSize: 14, fontWeight: 600 }}>Memory health</div>
          <div style={{ fontSize: 12.5, color: tone.color, marginTop: 2 }}>{tone.label}</div>
        </div>
        {health.anomalies.length > 0 && (
          <div style={{ marginLeft: "auto" }}>
            <Badge tone="outline">
              <span style={{ color: "var(--err, #f85149)" }}>
                {health.anomalies.length} anomal{health.anomalies.length === 1 ? "y" : "ies"}
              </span>
            </Badge>
          </div>
        )}
      </Card>

      {/* Lifecycle counters */}
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        <CountCard label="live" value={health.counts.live} />
        <CountCard label="low signal" value={health.counts.low_signal} alert />
        <CountCard label="near duplicate" value={health.counts.near_duplicate} alert />
        <CountCard label="stale" value={health.counts.stale} alert />
        <CountCard label="contradictions" value={health.counts.contradictions} alert />
        <CountCard label="missing embeddings" value={health.counts.missing_embeddings} alert />
      </div>

      {/* Recommendations */}
      {health.recommendations.length > 0 && (
        <Card style={{ padding: "16px 20px" }}>
          <div style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase", letterSpacing: 0.4, color: "var(--text-dim)", marginBottom: 10 }}>
            Recommendations
          </div>
          <ul style={{ margin: 0, paddingLeft: 18, display: "flex", flexDirection: "column", gap: 6 }}>
            {health.recommendations.map((r, i) => (
              <li key={i} style={{ fontSize: 13, color: "var(--text)" }}>{r}</li>
            ))}
          </ul>
        </Card>
      )}

      <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
        {/* By source, with spike badges */}
        <Card style={{ padding: "16px 20px", flex: "1 1 280px" }}>
          <div style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase", letterSpacing: 0.4, color: "var(--text-dim)", marginBottom: 10 }}>
            <I.upload size={12} /> By source
          </div>
          {(health.by_source ?? []).length === 0 ? (
            <div style={{ fontSize: 12.5, color: "var(--text-dim)" }}>No memories yet.</div>
          ) : (
            (health.by_source ?? []).map(s => (
              <div key={s.name} style={{ display: "flex", alignItems: "center", gap: 8, padding: "5px 0", fontSize: 12.5, fontFamily: "var(--font-mono)" }}>
                <span style={{ color: "var(--text)" }}>{s.name}</span>
                {spikySources.has(s.name) && (
                  <Badge tone="outline"><span style={{ color: "var(--err, #f85149)" }}>volume spike</span></Badge>
                )}
                <span style={{ marginLeft: "auto", color: "var(--text-dim)" }}>{s.count.toLocaleString()}</span>
              </div>
            ))
          )}
        </Card>

        {/* Age histogram */}
        <Card style={{ padding: "16px 20px", flex: "1 1 280px" }}>
          <div style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase", letterSpacing: 0.4, color: "var(--text-dim)", marginBottom: 10 }}>
            <I.clock size={12} /> Age
          </div>
          {(health.age_histogram ?? []).map(b => (
            <div key={b.name} style={{ display: "flex", padding: "5px 0", fontSize: 12.5, fontFamily: "var(--font-mono)" }}>
              <span style={{ color: "var(--text)" }}>{b.name}</span>
              <span style={{ marginLeft: "auto", color: "var(--text-dim)" }}>{b.count.toLocaleString()}</span>
            </div>
          ))}
        </Card>

        {/* Sync rejections (7d) */}
        <Card style={{ padding: "16px 20px", flex: "1 1 280px" }}>
          <div style={{ fontSize: 12, fontWeight: 600, textTransform: "uppercase", letterSpacing: 0.4, color: "var(--text-dim)", marginBottom: 10 }}>
            <I.shield size={12} /> Sync rejections (7d)
          </div>
          {(health.sync_rejections ?? []).length === 0 ? (
            <div style={{ fontSize: 12.5, color: "var(--text-dim)" }}>No rejections in the last 7 days.</div>
          ) : (
            (health.sync_rejections ?? []).map(r => (
              <div key={r.rule} style={{ display: "flex", padding: "5px 0", fontSize: 12.5, fontFamily: "var(--font-mono)" }}>
                <span style={{ color: "var(--text)" }}>{r.rule}</span>
                <span style={{ marginLeft: "auto", color: "var(--text-dim)" }}>{r.count.toLocaleString()}</span>
              </div>
            ))
          )}
        </Card>
      </div>
    </div>
  );
}
