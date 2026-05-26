import React, { useState, type FormEvent } from "react";

import { Btn, Input, Card } from "@/ds/components";
import { I, AnchoredOSSLogo } from "@/ds/icons";
import { useToast } from "@/components/ui/toast";
import { api, setToken } from "@/lib/api";
import { PROJECT_CATEGORIES, type ProjectCategory, type OnboardingComplete } from "@/lib/types";

const CATEGORY_LABELS: Record<ProjectCategory, string> = {
  service: "Service",
  library: "Library",
  app: "App",
  infra: "Infra",
  experiment: "Experiment",
  other: "Other",
};

const CATEGORY_ICONS: Record<ProjectCategory, React.ReactNode> = {
  service: <I.layers size={13} />,
  library: <I.cube size={13} />,
  app: <I.folder size={13} />,
  infra: <I.shield size={13} />,
  experiment: <I.activity size={13} />,
  other: <I.folder size={13} />,
};

interface ProjectRow {
  name: string;
  category: ProjectCategory;
}

function slugify(s: string): string {
  return s.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9-]/g, "").replace(/-+/g, "-").replace(/^-|-$/g, "");
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <span style={{
      fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)",
      letterSpacing: 0.4, textTransform: "uppercase" as const,
    }}>
      {children}
    </span>
  );
}

function FieldRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div style={{ marginBottom: 6 }}><Label>{label}</Label></div>
      {children}
    </div>
  );
}

function StepIndicator({ step }: { step: number }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 0 }}>
      {[1, 2, 3, 4].map((n, i) => {
        const done = n < step;
        const active = n === step;
        return (
          <React.Fragment key={n}>
            <div style={{
              width: 26, height: 26, borderRadius: "50%",
              background: done ? "var(--ok)" : active ? "var(--accent)" : "var(--bg-2)",
              border: `1px solid ${done ? "var(--ok)" : active ? "var(--accent-border)" : "var(--border)"}`,
              display: "inline-flex", alignItems: "center", justifyContent: "center",
              fontFamily: "var(--font-mono)", fontSize: 11, fontWeight: 600,
              color: done || active ? "#fff" : "var(--text-dim)",
              transition: "background .2s, color .2s, border-color .2s",
              flex: "none",
            }}>
              {done ? <I.check size={13} /> : n}
            </div>
            {i < 3 && (
              <div style={{
                width: 28, height: 2,
                background: n < step ? "var(--ok)" : "var(--border)",
                margin: "0 4px",
              }} />
            )}
          </React.Fragment>
        );
      })}
    </div>
  );
}

function CategoryPill({ value, onChange }: { value: ProjectCategory; onChange: (v: ProjectCategory) => void }) {
  const [open, setOpen] = useState(false);
  const ref = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    if (open) document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        style={{
          height: 36, minWidth: 130, padding: "0 10px",
          background: "var(--bg-1)", border: "1px solid var(--border)",
          borderRadius: "var(--radius)", color: "var(--text)",
          fontFamily: "inherit", fontSize: 13, cursor: "pointer",
          display: "inline-flex", alignItems: "center", gap: 8,
          justifyContent: "space-between",
        }}
      >
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
          {CATEGORY_ICONS[value]}
          {CATEGORY_LABELS[value]}
        </span>
        <I.chevD size={13} />
      </button>
      {open && (
        <div style={{
          position: "absolute", top: 40, right: 0, zIndex: 20,
          minWidth: 150, background: "var(--bg-1)",
          border: "1px solid var(--border)", borderRadius: "var(--radius)",
          boxShadow: "0 8px 24px rgba(0,0,0,.4)",
          padding: 4, display: "flex", flexDirection: "column",
        }}>
          {PROJECT_CATEGORIES.map(c => (
            <button
              key={c}
              type="button"
              onClick={() => { onChange(c); setOpen(false); }}
              style={{
                background: c === value ? "var(--accent-bg)" : "transparent",
                color: c === value ? "var(--accent)" : "var(--text)",
                border: 0, padding: "8px 10px", borderRadius: "var(--radius-sm)",
                cursor: "pointer", fontSize: 13, textAlign: "left" as const,
                display: "flex", alignItems: "center", gap: 8,
              }}
            >
              {CATEGORY_ICONS[c]}
              {CATEGORY_LABELS[c]}
              {c === value && <span style={{ marginLeft: "auto" }}><I.check size={12} /></span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function OnboardingPage() {
  const toast = useToast();

  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);

  const [orgName, setOrgName] = useState("");
  const [orgSlug, setOrgSlug] = useState("");

  const [adminName, setAdminName] = useState("");
  const [adminEmail, setAdminEmail] = useState("");
  const [adminPassword, setAdminPassword] = useState("");

  const [projects, setProjects] = useState<ProjectRow[]>([{ name: "", category: "service" }]);

  const [result, setResult] = useState<OnboardingComplete | null>(null);

  function handleOrgNameChange(v: string) {
    setOrgName(v);
    setOrgSlug(slugify(v));
  }

  function addProject() {
    if (projects.length >= 10) return;
    setProjects(ps => [...ps, { name: "", category: "service" }]);
  }

  function removeProject(i: number) {
    setProjects(ps => ps.filter((_, idx) => idx !== i));
  }

  function updateProject<K extends keyof ProjectRow>(i: number, field: K, value: ProjectRow[K]) {
    setProjects(ps => ps.map((p, idx) => idx === i ? { ...p, [field]: value } : p));
  }

  async function submitOnboarding(e: FormEvent) {
    e.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    try {
      const validProjects = projects.filter(p => p.name.trim());
      const res = await api.completeOnboarding({
        org: { name: orgName.trim(), slug: orgSlug.trim() || slugify(orgName) },
        admin: { email: adminEmail.trim(), password: adminPassword, display_name: adminName.trim() },
        projects: validProjects.map(p => ({ name: p.name.trim(), category: p.category })),
      });
      setToken(res.api_key);
      setResult(res);
      setStep(4);
    } catch (err: unknown) {
      const status = (err as { status?: number }).status;
      if (status === 409) {
        toast.push({ title: "Already bootstrapped", description: "This instance is already set up. Redirecting to login.", variant: "info" });
        window.location.assign("/login");
        return;
      }
      const msg = err instanceof Error ? err.message : "Setup failed";
      toast.push({ title: "Setup failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  // Full page reload — AuthProvider only re-reads localStorage on mount, so
  // a SPA navigate keeps `me=null` and RequireAuth bounces back to /login.
  function finish() {
    window.location.assign("/dashboard");
  }

  // Step 4 is its own centered single-column layout. Steps 1–3 use the
  // split hero/form layout.
  if (step === 4 && result) {
    return <ConnectStep result={result} onFinish={finish} />;
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 540px", minHeight: "100vh" }}>
      <HeroLeft step={step} />

      <div style={{
        display: "flex", alignItems: "center", justifyContent: "center",
        padding: "40px 56px", background: "var(--bg-1)",
        borderLeft: "1px solid var(--border)", overflowY: "auto",
      }}>
        <div style={{ width: "100%", maxWidth: 420 }}>

          {step === 1 && (
            <form onSubmit={e => { e.preventDefault(); if (orgName.trim()) setStep(2); }}>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.5, textTransform: "uppercase" as const, marginBottom: 14 }}>
                [ step 1 of 4 ]
              </div>
              <h2 style={{ fontSize: 28, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 6px", lineHeight: 1.1 }}>
                Name your organisation
              </h2>
              <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 28px", lineHeight: 1.55 }}>
                This will be the root of all your projects and API keys.
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
                <FieldRow label="Organisation name">
                  <Input full size="lg" placeholder="Acme Corp" value={orgName}
                    onChange={e => handleOrgNameChange(e.target.value)} required autoFocus />
                </FieldRow>
                <FieldRow label="Slug (URL-safe ID)">
                  <Input full size="lg" placeholder="acme-corp" value={orgSlug}
                    onChange={e => setOrgSlug(e.target.value)} required mono />
                </FieldRow>
                <Btn variant="primary" size="lg" full iconR={<I.arrowR />} style={{ marginTop: 8 }}>
                  Next — Admin account
                </Btn>
              </div>
            </form>
          )}

          {step === 2 && (
            <form onSubmit={e => { e.preventDefault(); if (adminName.trim() && adminEmail.trim() && adminPassword.length >= 8) setStep(3); }}>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.5, textTransform: "uppercase" as const, marginBottom: 14 }}>
                [ step 2 of 4 ]
              </div>
              <h2 style={{ fontSize: 28, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 6px", lineHeight: 1.1 }}>
                Admin account
              </h2>
              <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 28px", lineHeight: 1.55 }}>
                This will be your <span style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>admin</span> account — keep the credentials safe.
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
                <FieldRow label="Your name">
                  <Input full size="lg" placeholder="Jane Doe" value={adminName}
                    onChange={e => setAdminName(e.target.value)} required autoFocus autoComplete="name" />
                </FieldRow>
                <FieldRow label="Email">
                  <Input full size="lg" type="email" placeholder="jane@acme.com" value={adminEmail}
                    onChange={e => setAdminEmail(e.target.value)} required autoComplete="email" />
                </FieldRow>
                <div>
                  <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 6 }}>
                    <Label>Password</Label>
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: adminPassword.length > 0 && adminPassword.length < 8 ? "var(--err)" : "var(--text-dim)" }}>
                      min 8 chars
                    </span>
                  </div>
                  <Input full size="lg" type="password" placeholder="••••••••••••" value={adminPassword}
                    onChange={e => setAdminPassword(e.target.value)} required autoComplete="new-password" />
                </div>
                <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
                  <Btn variant="outline" size="lg" icon={<I.chevL />} onClick={() => setStep(1)} type="button">
                    Back
                  </Btn>
                  <Btn variant="primary" size="lg" full iconR={<I.arrowR />}>
                    Next — Projects
                  </Btn>
                </div>
              </div>
            </form>
          )}

          {step === 3 && (
            <form onSubmit={submitOnboarding}>
              <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.5, textTransform: "uppercase" as const, marginBottom: 14 }}>
                [ step 3 of 4 ]
              </div>
              <h2 style={{ fontSize: 28, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 6px", lineHeight: 1.1 }}>
                Add projects
              </h2>
              <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 20px", lineHeight: 1.55 }}>
                Optional — you can add more later. Max 10 at setup.
              </p>

              <div style={{ display: "flex", flexDirection: "column", gap: 10, marginBottom: 14 }}>
                {projects.map((p, i) => (
                  <div key={i} style={{ display: "flex", gap: 8, alignItems: "center" }}>
                    <Input
                      style={{ flex: 1 }}
                      size="md"
                      placeholder={`Project ${i + 1} name`}
                      value={p.name}
                      onChange={e => updateProject(i, "name", e.target.value)}
                    />
                    <CategoryPill
                      value={p.category}
                      onChange={c => updateProject(i, "category", c)}
                    />
                    <button
                      type="button"
                      onClick={() => removeProject(i)}
                      disabled={projects.length === 1}
                      style={{
                        width: 36, height: 36, display: "inline-flex", alignItems: "center",
                        justifyContent: "center", background: "transparent",
                        border: "1px solid var(--border)", borderRadius: "var(--radius)",
                        color: projects.length === 1 ? "var(--text-ghost)" : "var(--text-dim)",
                        cursor: projects.length === 1 ? "not-allowed" : "pointer",
                        flex: "none",
                      }}
                      aria-label="Remove project"
                    >
                      <I.x size={14} />
                    </button>
                  </div>
                ))}
              </div>

              {projects.length < 10 && (
                <Btn type="button" variant="outline" size="md" icon={<I.plus />} onClick={addProject} style={{ marginBottom: 24 }}>
                  Add another project
                </Btn>
              )}

              <div style={{ display: "flex", gap: 8 }}>
                <Btn variant="outline" size="lg" icon={<I.chevL />} onClick={() => setStep(2)} type="button">
                  Back
                </Btn>
                <Btn variant="primary" size="lg" full iconR={<I.arrowR />} style={{ opacity: submitting ? 0.7 : 1 }}>
                  {submitting ? "Setting up…" : "Complete setup"}
                </Btn>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}

function HeroLeft({ step }: { step: number }) {
  return (
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
      <div style={{ position: "relative", display: "flex", flexDirection: "column", alignItems: "center", gap: 28 }}>
        <svg viewBox="0 0 280 220" style={{ width: 260, height: 190 }}>
          <defs>
            <radialGradient id="onbGlow" cx="50%" cy="55%" r="50%">
              <stop offset="0" stopColor="var(--accent)" stopOpacity="0.18" />
              <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
            </radialGradient>
          </defs>
          <circle cx="140" cy="125" r="110" fill="url(#onbGlow)" />
          <polygon points="140,50 200,80 200,140 140,170 80,140 80,80" fill="var(--text)" opacity="0.12" />
          <polygon points="140,50 200,80 140,110 80,80" fill="var(--accent)" opacity="0.8" />
          <polygon points="80,80 140,110 140,170 80,140" fill="var(--text)" opacity="0.45" />
          <polygon points="200,80 140,110 140,170 200,140" fill="var(--accent)" opacity="0.55" />
          <line x1="80" y1="80" x2="140" y2="110" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
          <line x1="200" y1="80" x2="140" y2="110" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
          <line x1="140" y1="110" x2="140" y2="170" stroke="var(--bg)" strokeWidth="0.6" opacity="0.4" />
          <ellipse cx="140" cy="195" rx="56" ry="6" fill="var(--bg)" opacity="0.6" />
        </svg>
        <div style={{ textAlign: "center", maxWidth: 320 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--accent)", letterSpacing: 0.5, marginBottom: 16 }}>
            ── setup wizard ──
          </div>
          <StepIndicator step={step} />
          <div style={{ fontSize: 13, color: "var(--text-muted)", lineHeight: 1.6, marginTop: 18 }}>
            {step === 1 && "Name your organisation"}
            {step === 2 && "Set up your admin account"}
            {step === 3 && "Add your first projects"}
            {step === 4 && "Connect the CLI"}
          </div>
        </div>
      </div>
      <div style={{
        position: "relative", display: "flex", justifyContent: "space-between",
        fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--text-dim)",
      }}>
        <span>anchoredoss.dev</span>
        <span>step {step} of 4</span>
      </div>
    </div>
  );
}

// Robust clipboard copy that works on both secure and insecure origins.
// navigator.clipboard.writeText is only available over HTTPS / localhost;
// HTTP IPs (like the openclaw-gateway) need the document.execCommand fallback.
async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* fall through to fallback */ }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "0";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, text.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

// ConnectStep is shown after a successful onboarding. Single centered column
// with one key card and one terminal-style snippet block — keeps it scannable.
function ConnectStep({ result, onFinish }: { result: OnboardingComplete; onFinish: () => void }) {
  const [revealKey, setRevealKey] = useState(false);
  const origin = window.location.origin;
  const installCmd = "curl -fsSL https://anchoredoss.dev/install | bash";
  const configureCmd = `anchored remote configure --server ${origin} --key ${result.api_key}`;
  const linkCmds = result.projects.map(p => `anchored remote link ${p.id}`);

  const scriptLines: { kind: "comment" | "cmd"; text: string }[] = [
    { kind: "comment", text: "1) Install the Anchored CLI / MCP" },
    { kind: "cmd", text: installCmd },
    { kind: "comment", text: "2) Wire it to this server" },
    { kind: "cmd", text: configureCmd },
  ];
  if (linkCmds.length > 0) {
    scriptLines.push({ kind: "comment", text: "3) Subscribe to your projects (first link is the sync default)" });
    for (const c of linkCmds) scriptLines.push({ kind: "cmd", text: c });
    scriptLines.push({ kind: "comment", text: "4) Push your memories" });
  } else {
    scriptLines.push({ kind: "comment", text: "3) Push your memories" });
  }
  scriptLines.push({ kind: "cmd", text: "anchored remote sync" });

  const fullScript = scriptLines
    .map(l => (l.kind === "comment" ? `# ${l.text}` : l.text))
    .join("\n");

  return (
    <div style={{
      minHeight: "100vh", background: "var(--bg)",
      display: "flex", flexDirection: "column", alignItems: "center",
      padding: "40px 32px 60px",
    }}>
      <div style={{ width: "100%", maxWidth: 720 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 32 }}>
          <AnchoredOSSLogo size={22} />
          <StepIndicator step={4} />
        </div>

        <div style={{ textAlign: "center", marginBottom: 28 }}>
          <div style={{
            display: "inline-flex", alignItems: "center", gap: 8,
            background: "var(--ok-bg)", color: "var(--ok)",
            border: "1px solid var(--ok-border, var(--border))",
            padding: "4px 10px", borderRadius: 999,
            fontFamily: "var(--font-mono)", fontSize: 11, letterSpacing: 0.4,
            textTransform: "uppercase" as const, marginBottom: 16,
          }}>
            <I.check size={12} /> setup complete
          </div>
          <h1 style={{ fontSize: 30, fontWeight: 500, letterSpacing: -0.8, margin: "0 0 8px", lineHeight: 1.1 }}>
            Connect the CLI
          </h1>
          <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0, lineHeight: 1.55 }}>
            Save your API key — you won't see it again. Then run the snippet on your dev machine.
          </p>
        </div>

        {/* API Key — single compact row */}
        <Card style={{ padding: 14, marginBottom: 20 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 10 }}>
            <span style={{ color: "var(--warn)", display: "inline-flex" }}><I.key size={13} /></span>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--warn)", letterSpacing: 0.4, textTransform: "uppercase" as const, flex: 1 }}>
              Admin API key — save this now
            </span>
            <button type="button" onClick={() => setRevealKey(v => !v)} style={{
              background: "transparent", border: "1px solid var(--border)",
              borderRadius: "var(--radius)", color: "var(--text-dim)",
              cursor: "pointer", fontSize: 11, fontFamily: "var(--font-mono)",
              padding: "3px 8px", display: "inline-flex", alignItems: "center", gap: 4,
            }}>
              {revealKey ? <I.eyeOff size={12} /> : <I.eye size={12} />}
              {revealKey ? "hide" : "reveal"}
            </button>
          </div>
          <div style={{
            display: "flex", alignItems: "center", gap: 0,
            background: "var(--bg-1)", border: "1px solid var(--border)",
            borderRadius: "var(--radius)",
          }}>
            <code style={{
              flex: 1, padding: "10px 14px",
              fontFamily: "var(--font-mono)", fontSize: 12.5, color: "var(--text)",
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
            }}>
              {revealKey ? result.api_key : maskKey(result.api_key)}
            </code>
            <CopyButton text={result.api_key} label="copy key" />
          </div>
        </Card>

        {/* Single terminal block with the full setup sequence */}
        <div style={{ marginBottom: 20 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const }}>
              Run on your dev machine
            </div>
            <CopyButton text={fullScript} label="copy all" inline />
          </div>
          <Terminal lines={scriptLines} />
        </div>

        {result.projects.length > 0 && (
          <Card style={{ padding: 16, marginBottom: 28 }}>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 4 }}>
              Subscribe to your projects
            </div>
            <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 12 }}>
              Run <code style={{ fontFamily: "var(--font-mono)", color: "var(--text)" }}>anchored remote link &lt;project_id&gt;</code> for every project whose memories should sync from this machine.
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {result.projects.map(p => {
                const linkCmd = `anchored remote link ${p.id}`;
                return (
                  <div key={p.id} style={{
                    display: "flex", flexDirection: "column", gap: 6,
                    padding: 10, background: "var(--bg-1)",
                    border: "1px solid var(--border)", borderRadius: "var(--radius)",
                  }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "0 2px" }}>
                      <span style={{ color: "var(--text-muted)", display: "inline-flex" }}>
                        {CATEGORY_ICONS[p.category as ProjectCategory] ?? <I.folder size={13} />}
                      </span>
                      <span style={{ fontSize: 13, fontWeight: 500, color: "var(--text)" }}>{p.name}</span>
                      <code style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>
                        {p.slug}
                      </code>
                    </div>
                    <div style={{
                      display: "flex", alignItems: "stretch",
                      background: "var(--bg-2)", border: "1px solid var(--border)",
                      borderRadius: "var(--radius)", overflow: "hidden",
                    }}>
                      <code style={{
                        flex: 1, padding: "8px 12px",
                        fontFamily: "var(--font-mono)", fontSize: 12.5, color: "var(--text)",
                        whiteSpace: "nowrap", overflowX: "auto",
                      }}>
                        <span style={{ color: "var(--accent)" }}>$</span> {linkCmd}
                      </code>
                      <CopyButton text={linkCmd} label="copy" />
                    </div>
                  </div>
                );
              })}
            </div>
          </Card>
        )}

        <div style={{ display: "flex", justifyContent: "center" }}>
          <Btn variant="primary" size="lg" iconR={<I.arrowR />} onClick={onFinish}>
            Go to dashboard
          </Btn>
        </div>
      </div>
    </div>
  );
}

// Terminal mimics a real shell session — comments dim, commands with $ prompt.
function Terminal({ lines }: { lines: { kind: "comment" | "cmd"; text: string }[] }) {
  return (
    <div style={{
      background: "var(--bg-1)", border: "1px solid var(--border)",
      borderRadius: "var(--radius)", overflow: "hidden",
    }}>
      <div style={{
        background: "var(--bg-2)", borderBottom: "1px solid var(--border)",
        padding: "8px 14px", display: "flex", alignItems: "center", gap: 6,
        fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)",
      }}>
        <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#ff5f57" }} />
        <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#febc2e" }} />
        <span style={{ width: 9, height: 9, borderRadius: "50%", background: "#28c840" }} />
        <span style={{ marginLeft: 10 }}>~/anchored</span>
      </div>
      <pre style={{
        margin: 0, padding: "14px 16px",
        fontFamily: "var(--font-mono)", fontSize: 12.5, lineHeight: 1.7,
        color: "var(--text)", whiteSpace: "pre", overflowX: "auto", overflowY: "hidden",
      }}>
        {lines.map((l, i) => {
          if (l.kind === "comment") {
            return (
              <div key={i} style={{ color: "var(--text-dim)", marginTop: i === 0 ? 0 : 6 }}>
                # {l.text}
              </div>
            );
          }
          return (
            <div key={i} style={{ color: "var(--text)" }}>
              <span style={{ color: "var(--accent)" }}>$</span> {l.text}
            </div>
          );
        })}
      </pre>
    </div>
  );
}

// CopyButton: small inline button that uses the robust clipboard helper above.
function CopyButton({ text, label = "copy", inline = false }: { text: string; label?: string; inline?: boolean }) {
  const [state, setState] = useState<"idle" | "ok" | "err">("idle");
  async function onClick() {
    const ok = await copyToClipboard(text);
    setState(ok ? "ok" : "err");
    setTimeout(() => setState("idle"), 1800);
  }
  const color = state === "ok" ? "var(--ok)" : state === "err" ? "var(--err)" : "var(--text-dim)";
  return (
    <button type="button" onClick={onClick} style={{
      background: inline ? "transparent" : "transparent",
      border: inline ? "1px solid var(--border)" : 0,
      borderLeft: inline ? "1px solid var(--border)" : "1px solid var(--border)",
      borderRadius: inline ? "var(--radius)" : 0,
      color, cursor: "pointer",
      padding: inline ? "4px 10px" : "10px 14px",
      fontFamily: "var(--font-mono)", fontSize: 11,
      display: "inline-flex", alignItems: "center", gap: 5,
      alignSelf: "stretch",
      transition: "color .15s, background .15s",
    }}>
      {state === "ok" ? <I.check size={12} /> : state === "err" ? <I.x size={12} /> : <I.copy size={12} />}
      {state === "ok" ? "copied!" : state === "err" ? "failed" : label}
    </button>
  );
}

function maskKey(key: string): string {
  if (key.length < 16) return "•".repeat(key.length);
  const prefix = key.slice(0, 12);
  const suffix = key.slice(-4);
  return `${prefix}${"•".repeat(28)}${suffix}`;
}
