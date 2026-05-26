import { useState, type FormEvent } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";

import { Btn, Input } from "@/ds/components";
import { I, AnchoredOSSLogo } from "@/ds/icons";
import { Status } from "@/ds/components";
import { useAuth } from "@/lib/auth";
import { useToast } from "@/components/ui/toast";

export function LoginPage() {
  const { me, login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (me) {
    const target = (location.state as { from?: { pathname: string } } | null)?.from?.pathname ?? "/projects";
    return <Navigate to={target} replace />;
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!email.trim() || !password) return;
    setSubmitting(true);
    try {
      await login(email.trim(), password);
      const target = (location.state as { from?: { pathname: string } } | null)?.from?.pathname ?? "/projects";
      navigate(target, { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : "Invalid email or password";
      toast.push({ title: "Sign-in failed", description: message, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 520px", minHeight: "100vh" }}>
      <div
        style={{
          position: "relative",
          padding: "32px 40px",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          background: "var(--bg)",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            position: "absolute",
            inset: 0,
            opacity: 0.45,
            pointerEvents: "none",
            backgroundImage:
              "radial-gradient(circle, color-mix(in srgb, var(--text) 7%, transparent) 1px, transparent 1px)",
            backgroundSize: "20px 20px",
            maskImage:
              "radial-gradient(ellipse 70% 60% at 50% 50%, #000 40%, transparent 100%)",
            WebkitMaskImage:
              "radial-gradient(ellipse 70% 60% at 50% 50%, #000 40%, transparent 100%)",
          }}
        />

        <div style={{ position: "relative" }}>
          <AnchoredOSSLogo size={22} />
        </div>

        <div
          style={{
            position: "relative",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: 32,
          }}
        >
          <svg viewBox="0 0 280 220" style={{ width: 320, height: 240 }}>
            <defs>
              <radialGradient id="authGlow" cx="50%" cy="55%" r="50%">
                <stop offset="0" stopColor="var(--accent)" stopOpacity="0.18" />
                <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
              </radialGradient>
            </defs>
            <circle cx="140" cy="125" r="110" fill="url(#authGlow)" />
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
            <div
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 12,
                color: "var(--accent)",
                letterSpacing: 0.5,
                marginBottom: 10,
              }}
            >
              ── persistent memory for agents ──
            </div>
            <div style={{ fontSize: 16, color: "var(--text-muted)", lineHeight: 1.6 }}>
              Project-scoped, privacy-first memory sync for AI coding agents.
            </div>
          </div>
        </div>

        <div
          style={{
            position: "relative",
            display: "flex",
            justifyContent: "space-between",
            fontFamily: "var(--font-mono)",
            fontSize: 11.5,
            color: "var(--text-dim)",
          }}
        >
          <span>anchoredoss.dev</span>
          <span style={{ display: "inline-flex", alignItems: "center", gap: 14 }}>
            <Status value="ok" label="server · online" />
          </span>
        </div>
      </div>

      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: "40px 56px",
          background: "var(--bg-1)",
          borderLeft: "1px solid var(--border)",
        }}
      >
        <div style={{ width: "100%", maxWidth: 380 }}>
          <div
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 11,
              color: "var(--text-dim)",
              letterSpacing: 0.5,
              textTransform: "uppercase" as const,
              marginBottom: 14,
            }}
          >
            [ sign in ]
          </div>
          <h2
            style={{
              fontSize: 32,
              fontWeight: 500,
              letterSpacing: -0.8,
              margin: "0 0 8px",
              lineHeight: 1.1,
            }}
          >
            Welcome back
          </h2>
          <p
            style={{
              fontSize: 14,
              color: "var(--text-muted)",
              margin: "0 0 32px",
              lineHeight: 1.55,
            }}
          >
            Sign in to your{" "}
            <span style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>
              organisation
            </span>{" "}
            to manage projects, sync policies and team memory.
          </p>

          <form onSubmit={onSubmit}>
            <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 22 }}>
              <span style={{ flex: 1, height: 1, background: "var(--border)" }} />
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10.5,
                  color: "var(--text-dim)",
                  letterSpacing: 0.5,
                  textTransform: "uppercase" as const,
                }}
              >
                with password
              </span>
              <span style={{ flex: 1, height: 1, background: "var(--border)" }} />
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              <div>
                <div
                  style={{
                    display: "flex",
                    alignItems: "baseline",
                    justifyContent: "space-between",
                    marginBottom: 6,
                  }}
                >
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      color: "var(--text-dim)",
                      letterSpacing: 0.4,
                      textTransform: "uppercase" as const,
                    }}
                  >
                    Email
                  </span>
                </div>
                <Input
                  full
                  size="lg"
                  type="email"
                  autoComplete="email"
                  placeholder="you@company.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>

              <div>
                <div
                  style={{
                    display: "flex",
                    alignItems: "baseline",
                    justifyContent: "space-between",
                    marginBottom: 6,
                  }}
                >
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      color: "var(--text-dim)",
                      letterSpacing: 0.4,
                      textTransform: "uppercase" as const,
                    }}
                  >
                    Password
                  </span>
                </div>
                <Input
                  full
                  size="lg"
                  type="password"
                  autoComplete="current-password"
                  placeholder="••••••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>

              <Btn
                variant="primary"
                size="lg"
                full
                iconR={<I.arrowR />}
                style={{ marginTop: 8 }}
              >
                {submitting ? "Signing in…" : "Continue"}
              </Btn>
            </div>
          </form>

          <div
            style={{
              marginTop: 32,
              paddingTop: 22,
              borderTop: "1px solid var(--border)",
              fontSize: 13,
              color: "var(--text-muted)",
              textAlign: "center",
            }}
          >
            Don't have an account?{" "}
            <Link
              to="/register"
              style={{ color: "var(--accent)", fontWeight: 500, textDecoration: "none" }}
            >
              Create one
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
