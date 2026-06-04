import React from "react";
import { Card, Status, Btn } from "@/ds/components";
import { I } from "@/ds/icons";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Health, UpdateCheckResponse } from "@/lib/types";

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

type UpdatePhase =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "checked"; data: UpdateCheckResponse }
  | { kind: "confirming"; data: UpdateCheckResponse }
  | { kind: "applying" }
  | { kind: "polling"; latestVersion: string; startedAt: number }
  | { kind: "success"; version: string }
  | { kind: "timeout" }
  | { kind: "error"; message: string };

function UpdateSection() {
  const [phase, setPhase] = React.useState<UpdatePhase>({ kind: "idle" });
  const pollRef = React.useRef<ReturnType<typeof setInterval> | null>(null);

  function stopPolling() {
    if (pollRef.current !== null) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  React.useEffect(() => {
    // Initial check on mount
    doCheck();
    return () => stopPolling();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function doCheck() {
    setPhase({ kind: "checking" });
    try {
      const data = await api.checkUpdate();
      setPhase({ kind: "checked", data });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to check for updates";
      setPhase({ kind: "error", message: msg });
    }
  }

  async function doApply(latestVersion: string) {
    setPhase({ kind: "applying" });
    try {
      await api.applyUpdate();
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to apply update";
      setPhase({ kind: "error", message: msg });
      return;
    }
    const startedAt = Date.now();
    setPhase({ kind: "polling", latestVersion, startedAt });
    pollRef.current = setInterval(async () => {
      const elapsed = Date.now() - startedAt;
      if (elapsed > 90_000) {
        stopPolling();
        setPhase({ kind: "timeout" });
        return;
      }
      try {
        const h = await api.getHealthz();
        if (h.version === latestVersion) {
          stopPolling();
          setPhase({ kind: "success", version: latestVersion });
        }
      } catch {
        // server restarting — keep polling
      }
    }, 3000);
  }

  if (phase.kind === "checking") {
    return (
      <div style={{ fontSize: 12.5, color: "var(--text-dim)", fontFamily: "var(--font-mono)" }}>
        Checking for updates…
      </div>
    );
  }

  if (phase.kind === "error") {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <div style={{ fontSize: 12.5, color: "var(--err)" }}>{phase.message}</div>
        <Btn variant="ghost" size="sm" onClick={doCheck}>Retry</Btn>
      </div>
    );
  }

  if (phase.kind === "checked" || phase.kind === "confirming") {
    const { data } = phase;
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 12.5 }}>
            <span style={{ color: "var(--text-dim)" }}>current: </span>
            <span style={{ color: "var(--text)" }}>v{data.current_version}</span>
          </div>
          {data.update_available && (
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 12.5 }}>
              <span style={{ color: "var(--text-dim)" }}>latest: </span>
              <span style={{ color: "var(--ok)" }}>v{data.latest_version}</span>
            </div>
          )}
          <Btn variant="ghost" size="sm" onClick={doCheck}>Check again</Btn>
        </div>

        {data.update_available ? (
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div style={{
              padding: "8px 14px",
              background: "var(--ok-bg)", border: "1px solid color-mix(in srgb, var(--ok) 25%, transparent)",
              borderRadius: "var(--radius)", fontSize: 12.5, color: "var(--text-muted)",
            }}>
              Update available: <strong style={{ color: "var(--text)" }}>v{data.current_version} → v{data.latest_version}</strong>
            </div>
            {phase.kind === "confirming" ? (
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ fontSize: 12.5, color: "var(--text-muted)" }}>Apply update now?</span>
                <Btn variant="primary" size="sm" onClick={() => doApply(data.latest_version)}>Confirm</Btn>
                <Btn variant="ghost" size="sm" onClick={() => setPhase({ kind: "checked", data })}>Cancel</Btn>
              </div>
            ) : (
              <Btn variant="outline" size="sm" icon={<I.refresh />}
                onClick={() => setPhase({ kind: "confirming", data })}>
                Update server
              </Btn>
            )}
          </div>
        ) : (
          <div style={{ fontSize: 12.5, color: "var(--ok)", display: "inline-flex", alignItems: "center", gap: 6 }}>
            <I.check size={13} /> Up to date
          </div>
        )}
      </div>
    );
  }

  if (phase.kind === "applying") {
    return (
      <div style={{ fontSize: 12.5, color: "var(--text-dim)", fontFamily: "var(--font-mono)" }}>
        Applying update…
      </div>
    );
  }

  if (phase.kind === "polling") {
    return (
      <div style={{ fontSize: 12.5, color: "var(--info)", fontFamily: "var(--font-mono)", display: "inline-flex", alignItems: "center", gap: 8 }}>
        <I.refresh size={13} />
        Updating… the server will restart. Waiting for v{phase.latestVersion}…
      </div>
    );
  }

  if (phase.kind === "success") {
    return (
      <div style={{ fontSize: 12.5, color: "var(--ok)", display: "inline-flex", alignItems: "center", gap: 6 }}>
        <I.check size={13} /> Updated to v{phase.version}
      </div>
    );
  }

  if (phase.kind === "timeout") {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <div style={{ fontSize: 12.5, color: "var(--warn)" }}>
          The server hasn't responded with the new version yet. The update may need a manual check.
        </div>
        <Btn variant="ghost" size="sm" onClick={doCheck}>Check now</Btn>
      </div>
    );
  }

  // idle — shouldn't render (initial check fires on mount), but just in case:
  return null;
}

export function HealthPage() {
  const { me } = useAuth();
  const [health, setHealth] = React.useState<Health | null>(null);
  const [loading, setLoading] = React.useState(true);
  const isAdmin = me?.scope === "admin";

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

      <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 14, marginBottom: 14 }}>
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

      {isAdmin && (
        <Card style={{ padding: 20 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, fontWeight: 500, marginBottom: 14 }}>server update</div>
          <UpdateSection />
        </Card>
      )}
    </div>
  );
}
