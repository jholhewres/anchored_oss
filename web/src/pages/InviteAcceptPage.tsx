import { useState, useEffect, type FormEvent } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";

import { Btn, Input } from "@/ds/components";
import { I, AnchoredOSSLogo } from "@/ds/icons";
import { Status } from "@/ds/components";
import { useToast } from "@/components/ui/toast";
import { api, setToken } from "@/lib/api";
import type { InviteAcceptInfo } from "@/lib/types";

export function InviteAcceptPage() {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const toast = useToast();

  const [info, setInfo] = useState<InviteAcceptInfo | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [loading, setLoading] = useState(true);

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!token) { setLoadError(true); setLoading(false); return; }
    api.getInviteByToken(token)
      .then(data => {
        if (!data.valid) { setLoadError(true); }
        else { setInfo(data); }
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false));
  }, [token]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!token || !password || password !== confirm) return;
    if (password.length < 8) {
      toast.push({ title: "Password too short", description: "Minimum 8 characters.", variant: "error" });
      return;
    }
    setSubmitting(true);
    try {
      const res = await api.acceptInvite(token, password);
      setToken(res.api_key);
      localStorage.setItem("anchored_first_login", "1");
      navigate("/dashboard", { replace: true });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to accept invite";
      toast.push({ title: "Invite error", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 520px", minHeight: "100vh" }}>
      {/* Left hero */}
      <div style={{
        position: "relative", padding: "32px 40px",
        display: "flex", flexDirection: "column", justifyContent: "space-between",
        background: "var(--bg)", overflow: "hidden",
      }}>
        <div style={{
          position: "absolute", inset: 0, opacity: 0.45, pointerEvents: "none",
          backgroundImage: "radial-gradient(circle, color-mix(in srgb, var(--text) 7%, transparent) 1px, transparent 1px)",
          backgroundSize: "20px 20px",
          maskImage: "radial-gradient(ellipse 70% 60% at 50% 50%, #000 40%, transparent 100%)",
          WebkitMaskImage: "radial-gradient(ellipse 70% 60% at 50% 50%, #000 40%, transparent 100%)",
        }} />
        <div style={{ position: "relative" }}><AnchoredOSSLogo size={22} /></div>
        <div style={{ position: "relative", display: "flex", flexDirection: "column", alignItems: "center", gap: 32 }}>
          <svg viewBox="0 0 280 220" style={{ width: 320, height: 240 }}>
            <defs>
              <radialGradient id="invGlow" cx="50%" cy="55%" r="50%">
                <stop offset="0" stopColor="var(--accent)" stopOpacity="0.18" />
                <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
              </radialGradient>
            </defs>
            <circle cx="140" cy="125" r="110" fill="url(#invGlow)" />
            <polygon points="140,50 200,80 200,140 140,170 80,140 80,80" fill="var(--text)" opacity="0.12" />
            <polygon points="140,50 200,80 140,110 80,80" fill="var(--accent)" opacity="0.8" />
            <polygon points="80,80 140,110 140,170 80,140" fill="var(--text)" opacity="0.45" />
            <polygon points="200,80 140,110 140,170 200,140" fill="var(--accent)" opacity="0.55" />
            <line x1="80" y1="80" x2="140" y2="110" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
            <line x1="200" y1="80" x2="140" y2="110" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
            <line x1="140" y1="110" x2="140" y2="170" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
            <ellipse cx="140" cy="195" rx="56" ry="6" fill="var(--bg)" opacity="0.6" />
          </svg>
          <div style={{ textAlign: "center", maxWidth: 360 }}>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--accent)", letterSpacing: 0.5, marginBottom: 10 }}>
              ── you've been invited ──
            </div>
            <div style={{ fontSize: 16, color: "var(--text-muted)", lineHeight: 1.6 }}>
              Set a password to activate your account and start collaborating.
            </div>
          </div>
        </div>
        <div style={{
          position: "relative", display: "flex", justifyContent: "space-between",
          fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)",
        }}>
          <span>anchoredoss.dev</span>
          <span style={{ display: "inline-flex", alignItems: "center", gap: 14 }}>
            <Status value="ok" label="server · online" />
          </span>
        </div>
      </div>

      {/* Right form */}
      <div style={{
        display: "flex", alignItems: "center", justifyContent: "center",
        padding: "40px 56px", background: "var(--bg-1)", borderLeft: "1px solid var(--border)",
      }}>
        <div style={{ width: "100%", maxWidth: 380 }}>
          {loading && (
            <div style={{ color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 13 }}>
              Verifying invite…
            </div>
          )}

          {!loading && loadError && (
            <div>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--err)", letterSpacing: 0.5, textTransform: "uppercase" as const, marginBottom: 14 }}>
                [ invalid invite ]
              </div>
              <h2 style={{ fontSize: 28, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 8px", lineHeight: 1.1 }}>
                Invite expired or invalid
              </h2>
              <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 24px", lineHeight: 1.55 }}>
                This invite link is no longer valid. Ask an admin to send a new one.
              </p>
              <Link to="/login" style={{ color: "var(--accent)", fontWeight: 500, textDecoration: "none", fontSize: 14 }}>
                Back to login
              </Link>
            </div>
          )}

          {!loading && !loadError && info && (
            <form onSubmit={onSubmit}>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.5, textTransform: "uppercase" as const, marginBottom: 14 }}>
                [ accept invite ]
              </div>
              <h2 style={{ fontSize: 28, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 6px", lineHeight: 1.1 }}>
                Welcome, {info.display_name}
              </h2>
              <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 28px", lineHeight: 1.55 }}>
                Set a password to activate your account.
              </p>

              <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                <div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
                    Name
                  </div>
                  <Input full size="lg" value={info.display_name} readOnly />
                </div>
                <div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
                    Email
                  </div>
                  <Input full size="lg" value={info.email} readOnly />
                </div>
                <div>
                  <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 6 }}>
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const }}>
                      Password
                    </span>
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: password.length > 0 && password.length < 8 ? "var(--err)" : "var(--text-dim)" }}>
                      min 8 chars
                    </span>
                  </div>
                  <Input full size="lg" type="password" autoComplete="new-password"
                    placeholder="••••••••••••" value={password}
                    onChange={e => setPassword(e.target.value)} required autoFocus />
                </div>
                <div>
                  <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 6 }}>
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const }}>
                      Confirm password
                    </span>
                    {confirm && password !== confirm && (
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--err)" }}>mismatch</span>
                    )}
                  </div>
                  <Input
                    full size="lg" type="password" autoComplete="new-password"
                    placeholder="••••••••••••" value={confirm}
                    onChange={e => setConfirm(e.target.value)} required
                    error={Boolean(confirm && password !== confirm)}
                  />
                </div>

                <Btn variant="primary" size="lg" full iconR={<I.arrowR />} style={{ marginTop: 8 }}>
                  {submitting ? "Activating…" : "Activate account"}
                </Btn>
              </div>

              <div style={{ marginTop: 28, paddingTop: 22, borderTop: "1px solid var(--border)", fontSize: 13, color: "var(--text-muted)", textAlign: "center" }}>
                Already have an account?{" "}
                <Link to="/login" style={{ color: "var(--accent)", fontWeight: 500, textDecoration: "none" }}>
                  Sign in
                </Link>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
