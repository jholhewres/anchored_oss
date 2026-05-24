import React from "react";
import {
  Btn,
  Card,
  InstallCmd,
  SectionLabel,
  Badge,
} from "@/ds/components";
import { I, AnchoredOSSLogo, AnchoredLogo } from "@/ds/icons";

export function LandingPage() {
  return (
    <div style={{ position: "relative", overflow: "hidden" }}>
      <div
        style={{
          position: "absolute",
          inset: 0,
          opacity: 0.55,
          pointerEvents: "none",
          backgroundImage:
            "radial-gradient(circle, color-mix(in srgb, var(--text) 7%, transparent) 1px, transparent 1px)",
          backgroundSize: "20px 20px",
          maskImage:
            "radial-gradient(ellipse 75% 55% at 50% 35%, #000 50%, transparent 100%)",
          WebkitMaskImage:
            "radial-gradient(ellipse 75% 55% at 50% 35%, #000 50%, transparent 100%)",
        }}
      />

      <div style={{ position: "relative" }}>
        <LandingHeader />

        {/* ── Hero ────────────────────────────────────────── */}
        <section
          style={{
            padding: "72px 64px 0",
            display: "grid",
            gridTemplateColumns: "1fr 1fr",
            gap: 32,
            alignItems: "center",
            minHeight: 720,
          }}
        >
          <div>
            <div
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 10,
                fontFamily: "var(--font-mono)",
                fontSize: 12,
                padding: "6px 12px 6px 8px",
                background: "var(--accent-bg)",
                color: "var(--accent)",
                border: "1px solid var(--accent-border)",
                borderRadius: 999,
                marginBottom: 28,
              }}
            >
              <span
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 6,
                }}
              >
                <span
                  style={{
                    width: 6,
                    height: 6,
                    background: "var(--accent)",
                    borderRadius: 3,
                    boxShadow:
                      "0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent)",
                  }}
                />
                v0.4.2
              </span>
              <span
                style={{
                  width: 1,
                  height: 12,
                  background: "var(--accent-border)",
                }}
              />
              <span>free · open source · self-hosted</span>
            </div>

            <h1
              style={{
                fontSize: 84,
                fontWeight: 500,
                lineHeight: 0.96,
                letterSpacing: -3,
                margin: "0 0 26px",
              }}
            >
              Anchor what
              <br />
              your agents
              <br />
              <span style={{ position: "relative" }}>
                learn.
                <span
                  style={{
                    position: "absolute",
                    left: 0,
                    right: "38%",
                    bottom: 8,
                    height: 8,
                    background: "var(--accent)",
                    zIndex: -1,
                    opacity: 0.9,
                  }}
                />
              </span>
            </h1>

            <p
              style={{
                fontSize: 19,
                lineHeight: 1.55,
                color: "var(--text-muted)",
                maxWidth: 540,
                margin: 0,
              }}
            >
              <strong style={{ color: "var(--text)", fontWeight: 500 }}>
                Anchored
              </strong>{" "}
              is an{" "}
              <strong style={{ color: "var(--text)", fontWeight: 500 }}>
                MCP memory server
              </strong>{" "}
              that gives your code agents durable project context across
              sessions, editors, and CLI workflows.
            </p>

            <div
              style={{
                marginTop: 36,
                display: "flex",
                flexDirection: "column",
                gap: 12,
                alignItems: "flex-start",
              }}
            >
              <InstallCmd accent />
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  flexWrap: "wrap",
                }}
              >
                <Btn
                  as="a"
                  href="https://github.com/jholhewres/anchored#quick-start"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="primary"
                  size="lg"
                  icon={<I.terminal />}
                  iconR={<I.arrowR />}
                >
                  Install anchored
                </Btn>
                <Btn
                  as="a"
                  href="https://github.com/jholhewres/anchored"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="ghost"
                  size="lg"
                  icon={<I.fileText />}
                >
                  Read the docs
                </Btn>
                <Btn
                  as="a"
                  href="https://github.com/jholhewres/anchored"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="ghost"
                  size="lg"
                  icon={<I.github />}
                >
                  Star on GitHub
                </Btn>
              </div>
            </div>
          </div>

          <div style={{ position: "relative", height: 620 }}>
            <HeroCube />
          </div>
        </section>

        {/* ── Compatibility band ──────────────────────────── */}
        <section style={{ padding: "88px 64px 0" }}>
          <div
            style={{
              padding: "36px 40px",
              border: "1px solid var(--border)",
              borderRadius: "var(--radius-lg)",
              background:
                "color-mix(in srgb, var(--bg-2) 60%, transparent)",
              position: "relative",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                flexWrap: "wrap",
                gap: 18,
                marginBottom: 22,
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 14,
                }}
              >
                <SectionLabel n="00" label="compatible with" />
                <Badge tone="accent" icon={<I.terminal />}>
                  via MCP
                </Badge>
              </div>
              <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 12,
                  color: "var(--text-dim)",
                }}
              >
                one memory · every MCP-aware tool you already use
              </div>
            </div>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(4, 1fr)",
                gap: 10,
              }}
            >
              {[
                { id: "claude", name: "Claude Code", mark: "cc", tone: "accent" as const },
                { id: "codex", name: "Codex", mark: "cx", tone: undefined },
                { id: "cursor", name: "Cursor", mark: "›_", tone: undefined },
                { id: "opencode", name: "OpenCode", mark: "</>", tone: undefined },
                { id: "copilot", name: "GitHub Copilot", mark: "✦", tone: undefined },
                { id: "windsurf", name: "Windsurf", mark: "~", tone: undefined },
                { id: "cline", name: "Cline", mark: "cl", tone: undefined },
                { id: "devin", name: "Devin", mark: "◉", tone: undefined },
              ].map((t) => (
                <ToolChip key={t.id} {...t} />
              ))}
            </div>
          </div>
        </section>

        {/* ── Anchored · features ─────────────────────────── */}
        <section
          id="features"
          style={{ padding: "120px 64px 0", scrollMarginTop: 80 }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "flex-end",
              justifyContent: "space-between",
              flexWrap: "wrap",
              gap: 18,
              marginBottom: 36,
            }}
          >
            <div>
              <SectionLabel n="01" label="anchored · the mcp" />
              <h2
                style={{
                  fontSize: 44,
                  fontWeight: 500,
                  letterSpacing: -1.2,
                  margin: "20px 0 14px",
                  lineHeight: 1.05,
                  maxWidth: 680,
                }}
              >
                A memory server that
                <br />
                <em style={{ fontStyle: "normal", color: "var(--accent)" }}>
                  lives where your code does.
                </em>
              </h2>
              <p
                style={{
                  fontSize: 16,
                  color: "var(--text-muted)",
                  maxWidth: 620,
                  margin: 0,
                  lineHeight: 1.6,
                }}
              >
                Open the editor or CLI you already use. Anchored quietly attaches
                as an MCP server and starts collecting context. Decisions,
                conventions, learnings — written back the moment they're made,
                retrieved the moment they matter.
              </p>
            </div>
            <div
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 12,
                color: "var(--text-dim)",
                textAlign: "right",
              }}
            >
              <div>apache-2.0 · 100% free</div>
              <div style={{ color: "var(--text-ghost)", marginTop: 4 }}>
                github.com/jholhewres/anchored
              </div>
            </div>
          </div>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1.3fr 1fr 1fr",
              gridTemplateRows: "auto auto",
              gap: 14,
            }}
          >
            <Card
              style={{
                padding: 32,
                gridColumn: "1 / 2",
                gridRow: "1 / 3",
                display: "flex",
                flexDirection: "column",
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  color: "var(--accent)",
                }}
              >
                <I.graph size={18} />
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 11,
                    letterSpacing: 0.5,
                    textTransform: "uppercase" as const,
                  }}
                >
                  [01] knowledge graph
                </span>
              </div>
              <div
                style={{
                  fontSize: 26,
                  fontWeight: 500,
                  letterSpacing: -0.6,
                  marginTop: 18,
                  marginBottom: 10,
                }}
              >
                Memories that link.
              </div>
              <div
                style={{
                  fontSize: 14.5,
                  color: "var(--text-muted)",
                  lineHeight: 1.6,
                  marginBottom: 22,
                  maxWidth: 380,
                }}
              >
                Every memory is a node. Decisions supersede each other,
                conventions govern projects, learnings link to constraints. Your
                agent doesn't just retrieve — it{" "}
                <em>traverses</em>.
              </div>
              <div style={{ flex: 1, minHeight: 200 }}>
                <MiniGraph />
              </div>
            </Card>

            {[
              {
                n: "02",
                i: <I.lock />,
                t: "Local-first",
                b: "On-device embeddings via ONNX. SQLite store. Nothing leaves your machine unless you tell it to.",
              },
              {
                n: "03",
                i: <I.terminal />,
                t: "MCP-native",
                b: "Registers as an MCP server with your editor or CLI. No daemon, no API keys, no auth flow.",
              },
              {
                n: "04",
                i: <I.layers />,
                t: "Memory stack",
                b: "Layered by scope — session, machine, user, project. Resolved from most specific outward.",
              },
              {
                n: "05",
                i: <I.shield />,
                t: "Sanitised",
                b: "Redaction and policy guardrails by default. Secrets and PII are scrubbed before they're stored.",
              },
            ].map((f) => (
              <Card key={f.t} style={{ padding: 24 }}>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    color: "var(--accent)",
                  }}
                >
                  <span style={{ display: "inline-flex" }}>
                    {React.cloneElement(f.i, { size: 16 })}
                  </span>
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      letterSpacing: 0.5,
                      textTransform: "uppercase" as const,
                      color: "var(--text-dim)",
                    }}
                  >
                    [{f.n}]
                  </span>
                </div>
                <div
                  style={{
                    fontSize: 18,
                    fontWeight: 500,
                    marginTop: 16,
                    marginBottom: 6,
                    letterSpacing: -0.3,
                  }}
                >
                  {f.t}
                </div>
                <div
                  style={{
                    fontSize: 13.5,
                    color: "var(--text-muted)",
                    lineHeight: 1.55,
                  }}
                >
                  {f.b}
                </div>
              </Card>
            ))}
          </div>
        </section>

        {/* ── Memory in layers — cascading cubes ──────────── */}
        <section style={{ padding: "140px 64px 0" }}>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1.2fr",
              gap: 64,
              alignItems: "center",
            }}
          >
            <div>
              <SectionLabel n="02" label="memory · in layers" />
              <h2
                style={{
                  fontSize: 44,
                  fontWeight: 500,
                  letterSpacing: -1.2,
                  margin: "20px 0 18px",
                  lineHeight: 1.05,
                }}
              >
                Five scopes.
                <br />
                One resolution.
              </h2>
              <p
                style={{
                  fontSize: 15.5,
                  color: "var(--text-muted)",
                  lineHeight: 1.6,
                  margin: 0,
                  maxWidth: 460,
                }}
              >
                Anchored resolves memory from the most specific layer outward —
                session, machine, user, project, org. Your agent gets the right
                context at the right moment, automatically.
              </p>
              <div
                style={{
                  marginTop: 28,
                  fontFamily: "var(--font-mono)",
                  fontSize: 12,
                  color: "var(--text-dim)",
                  display: "flex",
                  flexDirection: "column",
                  gap: 6,
                }}
              >
                <div>
                  <span style={{ color: "var(--accent)" }}>›</span> reads
                  cascade outward
                </div>
                <div>
                  <span style={{ color: "var(--accent)" }}>›</span> writes
                  land at most-specific scope
                </div>
                <div>
                  <span style={{ color: "var(--accent)" }}>›</span> policies
                  gate what syncs upstream
                </div>
              </div>
            </div>
            <CubeStack />
          </div>
        </section>

        {/* ── For teams: Anchored OSS ─────────────────────── */}
        <section
          id="oss"
          style={{ padding: "160px 64px 0", scrollMarginTop: 80 }}
        >
          <div style={{ textAlign: "center", marginBottom: 40 }}>
            <div
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 12,
                color: "var(--text-dim)",
                letterSpacing: 0.8,
                marginBottom: 16,
              }}
            >
              ──── for teams ────
            </div>
            <div
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 14,
                marginBottom: 20,
              }}
            >
              <AnchoredLogo size={36} wordmark={false} />
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 56,
                  fontWeight: 600,
                  letterSpacing: -1.6,
                  lineHeight: 1,
                }}
              >
                anchored
              </span>
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontWeight: 600,
                  fontSize: 28,
                  padding: "4px 14px",
                  background: "var(--accent-bg)",
                  color: "var(--accent)",
                  border: "1px solid var(--accent-border)",
                  borderRadius: 8,
                  letterSpacing: 1,
                  textTransform: "uppercase" as const,
                  lineHeight: 1,
                }}
              >
                oss
              </span>
            </div>
            <h2
              style={{
                fontSize: 32,
                fontWeight: 400,
                letterSpacing: -0.7,
                margin: "0 0 14px",
                lineHeight: 1.15,
                maxWidth: 720,
                marginInline: "auto",
              }}
            >
              Remote, shared memory for your team — running on{" "}
              <em style={{ fontStyle: "normal", color: "var(--accent)" }}>
                your
              </em>{" "}
              infra.
            </h2>
            <p
              style={{
                fontSize: 15,
                color: "var(--text-muted)",
                maxWidth: 580,
                margin: "0 auto",
                lineHeight: 1.6,
              }}
            >
              An optional self-hosted server that syncs <em>only</em> shared
              knowledge — decisions, conventions, learnings — between teammates'
              local Anchored installs. Pair it with Anchored when you're ready.
            </p>
          </div>

          <Card
            style={{
              padding: 0,
              overflow: "hidden",
              border: "1px solid var(--accent-border)",
              background:
                "color-mix(in srgb, var(--accent) 4%, var(--bg-2))",
            }}
          >
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 1.1fr",
                minHeight: 420,
              }}
            >
              {/* left — specs + ctas */}
              <div
                style={{
                  padding: "40px 44px",
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "space-between",
                  borderRight: "1px solid var(--accent-border)",
                }}
              >
                <div>
                  <div
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      color: "var(--accent)",
                      letterSpacing: 0.5,
                      textTransform: "uppercase" as const,
                      marginBottom: 18,
                    }}
                  >
                    [ what oss adds ]
                  </div>
                  <div
                    style={{
                      fontSize: 22,
                      fontWeight: 500,
                      letterSpacing: -0.4,
                      marginBottom: 14,
                      lineHeight: 1.25,
                      maxWidth: 440,
                    }}
                  >
                    A privacy-first remote for the shared knowledge your team
                    agrees on.
                  </div>
                  <div
                    style={{
                      fontSize: 14,
                      color: "var(--text-muted)",
                      lineHeight: 1.6,
                      marginBottom: 28,
                      maxWidth: 440,
                    }}
                  >
                    Run it next to Anchored when your team needs to share project
                    facts, decisions and learnings across every developer's agent
                    — without ever leaking machine-local context.
                  </div>

                  <div
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 14,
                      paddingTop: 22,
                      borderTop: "1px solid var(--accent-border)",
                    }}
                  >
                    {[
                      {
                        i: <I.brain />,
                        t: "Dream system",
                        b: "Server-side dedup, contradiction reconciliation and summarisation jobs that keep team knowledge tight.",
                      },
                      {
                        i: <I.layers />,
                        t: "Project-scoped sync",
                        b: "Facts · decisions · learnings · plans · summaries · knowledge-graph triples. Nothing else.",
                      },
                      {
                        i: <I.shield />,
                        t: "Privacy guardrails",
                        b: "Local paths, usernames, secrets, embeddings and personal context are blocked at the gate.",
                      },
                      {
                        i: <I.users />,
                        t: "Org → team → project",
                        b: "Access via API keys with admin · sync · readonly scopes. Permissions per project.",
                      },
                      {
                        i: <I.activity />,
                        t: "Audit log",
                        b: "Every read and write recorded, reviewable per project, per actor.",
                      },
                    ].map((f) => (
                      <div
                        key={f.t}
                        style={{
                          display: "grid",
                          gridTemplateColumns: "24px 1fr",
                          gap: 12,
                          alignItems: "flex-start",
                        }}
                      >
                        <span
                          style={{
                            color: "var(--accent)",
                            marginTop: 2,
                          }}
                        >
                          {React.cloneElement(f.i, { size: 15 })}
                        </span>
                        <div>
                          <div
                            style={{
                              fontSize: 13.5,
                              fontWeight: 500,
                              color: "var(--text)",
                              letterSpacing: -0.1,
                            }}
                          >
                            {f.t}
                          </div>
                          <div
                            style={{
                              fontSize: 12.5,
                              color: "var(--text-muted)",
                              lineHeight: 1.55,
                              marginTop: 2,
                            }}
                          >
                            {f.b}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <div
                  style={{
                    display: "flex",
                    gap: 10,
                    flexWrap: "wrap",
                    alignItems: "center",
                    marginTop: 32,
                  }}
                >
                  <Btn
                    as="a"
                    href="/register"
                    variant="primary"
                    size="md"
                    icon={<I.terminal />}
                    iconR={<I.arrowR />}
                  >
                    Self-host OSS
                  </Btn>
                  <Btn
                    as="a"
                    href="https://github.com/jholhewres/anchored_oss#docker-compose"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="ghost"
                    size="md"
                    icon={<I.fileText />}
                  >
                    Setup guide
                  </Btn>
                </div>
              </div>

              {/* right — sync diagram */}
              <div
                style={{
                  position: "relative",
                  background:
                    "color-mix(in srgb, var(--bg-1) 80%, transparent)",
                  padding: "24px 32px",
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "center",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    marginBottom: 14,
                  }}
                >
                  <div
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      color: "var(--text-dim)",
                      letterSpacing: 0.4,
                      textTransform: "uppercase" as const,
                    }}
                  >
                    [ how sync works ]
                  </div>
                  <div
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      color: "var(--text-dim)",
                    }}
                  >
                    local → remote
                  </div>
                </div>
                <OSSSyncDiagram />
              </div>
            </div>
          </Card>

          <div
            style={{
              marginTop: 22,
              display: "flex",
              justifyContent: "center",
              gap: 18,
              fontFamily: "var(--font-mono)",
              fontSize: 12,
              color: "var(--text-dim)",
              flexWrap: "wrap",
            }}
          >
            <span>
              <span style={{ color: "var(--ok)" }}>›</span> opt-in · totally
              separate from anchored
            </span>
            <span>
              <span style={{ color: "var(--ok)" }}>›</span> apache-2.0
            </span>
            <span>
              <span style={{ color: "var(--ok)" }}>›</span> works offline ·
              runs on your hardware
            </span>
          </div>
        </section>

        {/* ── Final CTA ───────────────────────────────────── */}
        <section style={{ padding: "160px 64px 0" }}>
          <div
            style={{
              position: "relative",
              borderTop: "1px solid var(--border)",
              borderBottom: "1px solid var(--border)",
              padding: "88px 0 96px",
              textAlign: "center",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                position: "absolute",
                inset: 0,
                opacity: 0.35,
                pointerEvents: "none",
                backgroundImage:
                  "radial-gradient(circle, color-mix(in srgb, var(--accent) 18%, transparent) 1px, transparent 1.5px)",
                backgroundSize: "24px 24px",
                maskImage:
                  "radial-gradient(ellipse 60% 70% at 50% 50%, #000 30%, transparent 80%)",
                WebkitMaskImage:
                  "radial-gradient(ellipse 60% 70% at 50% 50%, #000 30%, transparent 80%)",
              }}
            />
            <div style={{ position: "relative" }}>
              <h2
                style={{
                  fontSize: 64,
                  fontWeight: 500,
                  letterSpacing: -2,
                  margin: "0 0 18px",
                  lineHeight: 1.0,
                }}
              >
                Stop teaching your agent
                <br />
                <em style={{ fontStyle: "normal", color: "var(--accent)" }}>
                  the same thing twice.
                </em>
              </h2>
              <p
                style={{
                  fontSize: 16,
                  color: "var(--text-muted)",
                  maxWidth: 560,
                  margin: "0 auto 36px",
                  lineHeight: 1.55,
                }}
              >
                Install the local memory server, then add the self-hosted team
                layer when your project knowledge needs to travel across the
                whole engineering org.
              </p>
              <div
                style={{
                  display: "inline-flex",
                  gap: 12,
                  flexWrap: "wrap",
                  justifyContent: "center",
                  alignItems: "center",
                }}
              >
                <InstallCmd accent />
                <Btn
                  as="a"
                  href="/register"
                  variant="primary"
                  size="lg"
                  iconR={<I.arrowR />}
                >
                  Self-host OSS
                </Btn>
              </div>

              {/* Contribute strip */}
              <div
                style={{
                  width: "min(100%, 880px)",
                  margin: "44px auto 0",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 16,
                  padding: "14px 20px",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-lg)",
                  background:
                    "color-mix(in srgb, var(--bg-2) 60%, transparent)",
                  flexWrap: "wrap",
                }}
              >
                <I.github size={18} />
                <div style={{ textAlign: "left", flex: "1 1 460px" }}>
                  <div
                    style={{
                      fontSize: 13.5,
                      color: "var(--text)",
                      fontWeight: 500,
                    }}
                  >
                    Built in the open · contributions welcome
                  </div>
                  <div
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 11.5,
                      color: "var(--text-dim)",
                      marginTop: 2,
                    }}
                  >
                    github.com/jholhewres/anchored
                    <span
                      style={{ color: "var(--text-ghost)", margin: "0 8px" }}
                    >
                      ·
                    </span>
                    4.2k stars
                    <span
                      style={{ color: "var(--text-ghost)", margin: "0 8px" }}
                    >
                      ·
                    </span>
                    47 contributors
                    <span
                      style={{ color: "var(--text-ghost)", margin: "0 8px" }}
                    >
                      ·
                    </span>
                    good first issues open
                  </div>
                </div>
                <Btn
                  as="a"
                  href="https://github.com/jholhewres/anchored"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="outline"
                  size="md"
                  icon={<I.github />}
                  iconR={<I.arrowUR />}
                >
                  Contribute
                </Btn>
              </div>
            </div>
          </div>
        </section>

        <LandingFooter />
      </div>
    </div>
  );
}

// ─── LandingHeader ──────────────────────────────────────────
function LandingHeader() {
  return (
    <header
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "20px 64px",
        background: "transparent",
        position: "relative",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 36 }}>
        <AnchoredOSSLogo size={22} />
        <nav
          style={{
            display: "flex",
            gap: 22,
            fontSize: 13.5,
            color: "var(--text-muted)",
          }}
        >
          <a href="#features" style={{ color: "inherit", textDecoration: "none" }}>
            Features
          </a>
          <a href="#oss" style={{ color: "inherit", textDecoration: "none" }}>
            anchored OSS
          </a>
          <a
            href="https://github.com/jholhewres/anchored"
            target="_blank"
            rel="noopener noreferrer"
            style={{ color: "inherit", textDecoration: "none" }}
          >
            Docs
          </a>
        </nav>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <a
          href="https://github.com/jholhewres/anchored"
          target="_blank"
          rel="noopener noreferrer"
          aria-label="Star Anchored on GitHub"
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 6,
            fontSize: 13,
            color: "var(--text-muted)",
            textDecoration: "none",
          }}
        >
          <I.github size={14} /> star
        </a>
        <span style={{ width: 1, height: 18, background: "var(--border)" }} />
        <Btn as="a" href="#oss" variant="primary" size="sm" iconR={<I.arrowR />}>
          anchored OSS
        </Btn>
      </div>
    </header>
  );
}

// ─── LandingFooter ──────────────────────────────────────────
function LandingFooter() {
  return (
    <footer
      style={{
        padding: "64px 64px 36px",
        borderTop: "1px solid var(--border)",
        marginTop: 96,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "flex-end",
          justifyContent: "space-between",
          gap: 32,
          flexWrap: "wrap",
          marginBottom: 36,
        }}
      >
        <div style={{ maxWidth: 460 }}>
          <AnchoredOSSLogo size={22} />
          <div
            style={{
              fontSize: 14,
              color: "var(--text-muted)",
              marginTop: 14,
              lineHeight: 1.6,
            }}
          >
            A free, open-source MCP server that gives code agents persistent,
            project-scoped memory. Optional self-hosted team sync.
          </div>
        </div>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Btn
            as="a"
            href="https://github.com/jholhewres/anchored"
            target="_blank"
            rel="noopener noreferrer"
            variant="outline"
            size="md"
            icon={<I.github />}
            iconR={<I.arrowUR />}
          >
            github.com/jholhewres/anchored
          </Btn>
          <Btn
            as="a"
            href="https://github.com/jholhewres/anchored"
            target="_blank"
            rel="noopener noreferrer"
            variant="ghost"
            size="md"
            icon={<I.fileText />}
          >
            Docs
          </Btn>
        </div>
      </div>

      <div
        style={{
          paddingTop: 24,
          borderTop: "1px solid var(--border)",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 16,
          flexWrap: "wrap",
          fontFamily: "var(--font-mono)",
          fontSize: 11.5,
          color: "var(--text-dim)",
        }}
      >
        <div
          style={{ display: "inline-flex", alignItems: "center", gap: 8 }}
        >
          <span style={{ color: "var(--text-ghost)" }}>//</span>
          built by{" "}
          <a
            href="https://jhol.dev"
            target="_blank"
            rel="noopener noreferrer"
            style={{ color: "var(--text)", fontWeight: 500 }}
          >
            Jhol H.
          </a>
          <span style={{ color: "var(--text-ghost)" }}>·</span>
          <a
            href="https://jhol.dev"
            target="_blank"
            rel="noopener noreferrer"
            style={{ color: "var(--text-muted)" }}
          >
            jhol.dev
          </a>
        </div>
        <div
          style={{ display: "inline-flex", alignItems: "center", gap: 14 }}
        >
          <span style={{ color: "var(--text-ghost)" }}>apache-2.0</span>
          <span style={{ color: "var(--text-ghost)" }}>v0.4.2</span>
        </div>
      </div>
    </footer>
  );
}

// ─── ToolChip ───────────────────────────────────────────────
function ToolChip({
  name,
  mark,
  tone,
}: {
  name: string;
  mark: string;
  tone?: "accent";
}) {
  const accent = tone === "accent";
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "12px 14px",
        background: "var(--bg-2)",
        border: `1px solid ${accent ? "var(--accent-border)" : "var(--border)"}`,
        borderRadius: "var(--radius)",
      }}
    >
      <span
        style={{
          width: 32,
          height: 32,
          flex: "none",
          borderRadius: "var(--radius-sm)",
          background: accent ? "var(--accent-bg)" : "var(--bg-1)",
          border: `1px solid ${accent ? "var(--accent-border)" : "var(--border)"}`,
          color: accent ? "var(--accent)" : "var(--text)",
          fontFamily: "var(--font-mono)",
          fontSize: 12,
          fontWeight: 600,
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          letterSpacing: 0,
        }}
      >
        {mark}
      </span>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div
          style={{
            fontSize: 13.5,
            fontWeight: 500,
            color: "var(--text)",
            letterSpacing: -0.1,
          }}
        >
          {name}
        </div>
        <div
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 10.5,
            color: "var(--text-dim)",
            marginTop: 2,
            letterSpacing: 0.3,
          }}
        >
          MCP · supported
        </div>
      </div>
    </div>
  );
}

// ─── HeroCube ───────────────────────────────────────────────
function HeroCube() {
  return (
    <svg viewBox="0 0 560 620" style={{ width: "100%", height: "100%" }}>
      <defs>
        <radialGradient id="heroGlow" cx="50%" cy="55%" r="50%">
          <stop offset="0" stopColor="var(--accent)" stopOpacity="0.18" />
          <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
        </radialGradient>
      </defs>

      <circle cx="290" cy="340" r="220" fill="url(#heroGlow)" />

      <g opacity="0.25" stroke="var(--text)" strokeWidth="0.5" fill="none">
        {[0, 1, 2, 3, 4, 5, 6].map((i) => {
          const y = 470 + i * 18;
          const inset = i * 22;
          return (
            <line
              key={`fl${i}`}
              x1={60 + inset}
              y1={y}
              x2={520 - inset}
              y2={y}
            />
          );
        })}
        {[-3, -2, -1, 0, 1, 2, 3].map((i) => (
          <line
            key={`fv${i}`}
            x1={290 + i * 40}
            y1="470"
            x2={290 + i * 90}
            y2="600"
          />
        ))}
      </g>

      <g
        stroke="var(--accent)"
        strokeWidth="1"
        strokeDasharray="2 4"
        opacity="0.55"
        fill="none"
      >
        <path d="M 145 130 L 240 285" />
        <path d="M 460 130 L 335 285" />
        <path d="M 460 470 L 335 380" />
        <path d="M 145 470 L 240 380" />
      </g>

      <IsoCube cx={290} cy={335} size={155} />

      <ellipse
        cx="290"
        cy="500"
        rx="120"
        ry="14"
        fill="var(--bg)"
        opacity="0.6"
      />

      <Pin x={50} y={104} label="decision" detail="jwt.v5 migration" accent />
      <Pin x={350} y={104} label="convention" detail="snake_case claims" />
      <Pin x={350} y={488} label="learning" detail="webhook idempotency" />
      <Pin x={50} y={488} label="policy" detail="redact secrets" />

      <g transform="translate(290, 560)" textAnchor="middle">
        <text
          fontFamily="var(--font-mono)"
          fontSize="11"
          fill="var(--accent)"
          letterSpacing="0.5"
        >
          [ project · acme-api ]
        </text>
        <text
          y="18"
          fontFamily="var(--font-mono)"
          fontSize="10.5"
          fill="var(--text-dim)"
        >
          4,214 memories · 9,182 edges · synced 12s ago
        </text>
      </g>
    </svg>
  );
}

function Pin({
  x,
  y,
  label,
  detail,
  accent,
}: {
  x: number;
  y: number;
  label: string;
  detail: string;
  accent?: boolean;
}) {
  const c = accent ? "var(--accent)" : "var(--text-muted)";
  return (
    <g>
      <circle cx={x} cy={y} r="3" fill={c} />
      <circle
        cx={x}
        cy={y}
        r="7"
        fill="none"
        stroke={c}
        strokeWidth="0.5"
        opacity="0.6"
      />
      <text
        x={x + 14}
        y={y - 2}
        fontFamily="var(--font-mono)"
        fontSize="11"
        fill={c}
        fontWeight="500"
      >
        {label}
      </text>
      <text
        x={x + 14}
        y={y + 12}
        fontFamily="var(--font-mono)"
        fontSize="10.5"
        fill="var(--text-dim)"
      >
        {detail}
      </text>
    </g>
  );
}

// ─── CubeStack ──────────────────────────────────────────────
function CubeStack() {
  const layers = [
    { l: "session", count: "1 mem", size: 22, dim: true },
    { l: "machine", count: "28 mem", size: 32, dim: true },
    { l: "user", count: "142 mem", size: 44, dim: true },
    { l: "project", count: "4,214 mem", size: 64, accent: true },
    { l: "org", count: "14,832 mem", size: 84, dim: true },
  ];

  const GAP = 26;
  const PAD = 24;
  const BASELINE = 248;
  const halfW = (s: number) => s * 0.866;

  let cursor = PAD;
  const positions = layers.map((L) => {
    cursor += halfW(L.size);
    const x = cursor;
    cursor += halfW(L.size) + GAP;
    const cy = BASELINE - L.size;
    return { ...L, x, cy };
  });
  const totalW = cursor - GAP + PAD;
  const totalH = 340;

  return (
    <svg
      viewBox={`0 0 ${totalW} ${totalH}`}
      style={{ width: "100%", height: "auto" }}
    >
      <line
        x1={PAD}
        y1={BASELINE + 2}
        x2={totalW - PAD}
        y2={BASELINE + 2}
        stroke="var(--border)"
        strokeDasharray="2 4"
      />

      <g
        stroke="var(--text-ghost)"
        strokeWidth="1"
        strokeDasharray="3 4"
        fill="none"
      >
        {positions.slice(0, -1).map((p, i) => {
          const n = positions[i + 1];
          const y = BASELINE - Math.min(p.size, n.size) * 0.5;
          const x1 = p.x + halfW(p.size) + 4;
          const x2 = n.x - halfW(n.size) - 4;
          return <path key={i} d={`M ${x1} ${y} L ${x2} ${y}`} />;
        })}
      </g>

      {positions.map((p) => (
        <g key={p.l}>
          <IsoCube
            cx={p.x}
            cy={p.cy}
            size={p.size}
            dim={p.dim}
            accent={p.accent}
          />
        </g>
      ))}

      {positions.map((p) => (
        <g key={p.l + "-lab"}>
          <text
            x={p.x}
            y={BASELINE + 22}
            fontFamily="var(--font-mono)"
            fontSize="11"
            fill={p.accent ? "var(--accent)" : "var(--text-muted)"}
            fontWeight={p.accent ? 600 : 400}
            textAnchor="middle"
          >
            {p.l}
          </text>
          <text
            x={p.x}
            y={BASELINE + 38}
            fontFamily="var(--font-mono)"
            fontSize="10"
            fill="var(--text-dim)"
            textAnchor="middle"
          >
            {p.count}
          </text>
        </g>
      ))}

      <g
        fontFamily="var(--font-mono)"
        fontSize="10"
        fill="var(--text-dim)"
        letterSpacing="0.5"
      >
        <text x={PAD} y="22" textAnchor="start">
          [ MOST SPECIFIC ]
        </text>
        <text x={totalW - PAD} y="22" textAnchor="end">
          [ WIDEST ]
        </text>
      </g>
      <g stroke="var(--text-ghost)" strokeWidth="0.5" fill="none">
        <path
          d={`M ${PAD} 32 L ${PAD} 38 L ${totalW - PAD} 38 L ${totalW - PAD} 32`}
        />
        <line x1={totalW / 2} y1="38" x2={totalW / 2} y2="44" />
      </g>
      <text
        x={totalW / 2}
        y={56}
        fontFamily="var(--font-mono)"
        fontSize="10"
        fill="var(--text-dim)"
        letterSpacing="0.5"
        textAnchor="middle"
      >
        resolution direction →
      </text>
    </svg>
  );
}

// ─── MiniGraph (enhanced with 8 nodes + curved edges) ───────
function MiniGraph() {
  const nodes = [
    { id: "proj", x: 240, y: 175, type: "project", label: "acme-api", hub: true },
    { id: "d1", x: 100, y: 80, type: "decision", label: "jwt.v5", accent: true },
    { id: "c1", x: 380, y: 80, type: "convention", label: "snake_case claims" },
    { id: "p1", x: 60, y: 200, type: "pattern", label: "auth/token.go" },
    { id: "l1", x: 130, y: 290, type: "learning", label: "webhook idemp." },
    { id: "co1", x: 410, y: 290, type: "constraint", label: "no raw tokens" },
    { id: "p2", x: 430, y: 200, type: "pattern", label: "inline tests" },
    { id: "l2", x: 280, y: 50, type: "learning", label: "rs256 → eddsa" },
  ];
  const byId = Object.fromEntries(nodes.map((n) => [n.id, n]));

  const edges = [
    { a: "d1", b: "proj", kind: "decides" },
    { a: "d1", b: "l2", kind: "supersedes" },
    { a: "c1", b: "proj", kind: "governs" },
    { a: "p1", b: "proj", kind: "in" },
    { a: "p2", b: "proj", kind: "in" },
    { a: "l1", b: "proj", kind: "about" },
    { a: "co1", b: "proj", kind: "enforces" },
    { a: "l2", b: "proj", kind: "about" },
    { a: "p1", b: "p2", kind: "similar", soft: true },
    { a: "co1", b: "l1", kind: "led-to", soft: true },
  ];

  const typeColor: Record<string, string> = {
    project: "var(--accent)",
    decision: "var(--accent)",
    convention: "var(--info)",
    pattern: "var(--text-muted)",
    learning: "var(--warn)",
    constraint: "var(--err)",
  };
  const typeBg: Record<string, string> = {
    project: "var(--accent-bg)",
    decision: "var(--accent-bg)",
    convention: "var(--info-bg)",
    pattern: "var(--bg-3)",
    learning: "var(--warn-bg)",
    constraint: "var(--err-bg)",
  };

  const W = 500,
    H = 360;

  const curve = (
    ax: number,
    ay: number,
    bx: number,
    by: number,
    k = 0.18
  ) => {
    const mx = (ax + bx) / 2,
      my = (ay + by) / 2;
    const dx = bx - ax,
      dy = by - ay;
    const ox = -dy * k,
      oy = dx * k;
    return `M ${ax} ${ay} Q ${mx + ox} ${my + oy} ${bx} ${by}`;
  };

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: "100%", height: "100%" }}
    >
      <defs>
        <radialGradient id="kgGlow" cx="50%" cy="50%" r="50%">
          <stop offset="0" stopColor="var(--accent)" stopOpacity="0.18" />
          <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
        </radialGradient>
      </defs>
      <circle
        cx={byId.proj.x}
        cy={byId.proj.y}
        r="90"
        fill="url(#kgGlow)"
      />

      <g fill="none">
        {edges.map((e, i) => {
          const a = byId[e.a],
            b = byId[e.b];
          const stroke = e.soft
            ? "var(--text-ghost)"
            : "var(--border-strong)";
          const dash = e.soft ? "2 4" : "0";
          return (
            <g key={i}>
              <path
                d={curve(
                  a.x,
                  a.y,
                  b.x,
                  b.y,
                  e.soft ? 0.05 : 0.1
                )}
                stroke={stroke}
                strokeWidth="1"
                strokeDasharray={dash}
                opacity={e.soft ? 0.7 : 1}
              />
              {!e.soft && (
                <text
                  x={(a.x + b.x) / 2}
                  y={(a.y + b.y) / 2 - 4}
                  fontFamily="var(--font-mono)"
                  fontSize="8.5"
                  fill="var(--text-dim)"
                  textAnchor="middle"
                  opacity="0.9"
                >
                  {e.kind}
                </text>
              )}
            </g>
          );
        })}
      </g>

      <g>
        {nodes.map((n) => {
          const color = typeColor[n.type];
          const bg = typeBg[n.type];
          const r = n.hub ? 11 : 5;
          return (
            <g key={n.id}>
              <circle
                cx={n.x}
                cy={n.y}
                r={n.hub ? 22 : 14}
                fill={bg}
                stroke={color}
                strokeWidth="0.75"
                opacity={n.hub ? 1 : 0.85}
              />
              <circle cx={n.x} cy={n.y} r={r} fill={color} />
              <text
                x={n.x}
                y={n.y - (n.hub ? 30 : 22)}
                fontFamily="var(--font-mono)"
                fontSize="8.5"
                fill={color}
                textAnchor="middle"
                letterSpacing="0.4"
                style={{ textTransform: "uppercase" as const }}
              >
                {n.type}
              </text>
              <text
                x={n.x}
                y={n.y + (n.hub ? 38 : 28)}
                fontFamily="var(--font-mono)"
                fontSize={n.hub ? 11 : 10}
                fill={n.hub ? "var(--text)" : "var(--text-muted)"}
                fontWeight={n.hub ? 600 : 400}
                textAnchor="middle"
              >
                {n.label}
              </text>
            </g>
          );
        })}
      </g>

      <g fontFamily="var(--font-mono)" fontSize="9.5" fill="var(--text-dim)">
        <text x="12" y={H - 8}>
          {nodes.length} of 4,214 nodes
        </text>
        <text x={W - 12} y={H - 8} textAnchor="end">
          {edges.length} of 9,182 edges
        </text>
      </g>
    </svg>
  );
}

// ─── OSSSyncDiagram ─────────────────────────────────────────
function OSSSyncDiagram() {
  const W = 480;
  const H = 380;

  const COL_LEFT = 90;
  const REMOTE_Y = 60;
  const GATE_Y = 190;
  const LOCAL_Y = 296;
  const DEV_X = [140, 250, 360];

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: "100%", height: "auto", display: "block" }}
    >
      <defs>
        <radialGradient id="ossSrvGlow" cx="50%" cy="50%" r="50%">
          <stop
            offset="0"
            stopColor="var(--accent)"
            stopOpacity="0.22"
          />
          <stop
            offset="1"
            stopColor="var(--accent)"
            stopOpacity="0"
          />
        </radialGradient>
      </defs>

      <g
        fontFamily="var(--font-mono)"
        fontSize="9"
        fill="var(--text-dim)"
        letterSpacing="0.7"
        style={{ textTransform: "uppercase" as const }}
      >
        <text x={14} y={REMOTE_Y + 4}>
          [ remote ]
        </text>
        <text x={14} y={GATE_Y + 4}>
          [ policy ]
        </text>
        <text x={14} y={LOCAL_Y + 4}>
          [ local ]
        </text>
      </g>

      <g stroke="var(--border)" strokeDasharray="2 4">
        <line x1={COL_LEFT} y1={REMOTE_Y} x2={W - 14} y2={REMOTE_Y} />
        <line x1={COL_LEFT} y1={GATE_Y} x2={W - 14} y2={GATE_Y} />
        <line x1={COL_LEFT} y1={LOCAL_Y} x2={W - 14} y2={LOCAL_Y} />
      </g>

      {/* REMOTE: server */}
      <g>
        <circle
          cx={W / 2}
          cy={REMOTE_Y}
          r="68"
          fill="url(#ossSrvGlow)"
        />
        <IsoCube cx={W / 2} cy={REMOTE_Y - 4} size={28} />
        <g transform={`translate(${W / 2 + 56} ${REMOTE_Y - 22})`}>
          <rect
            x="0"
            y="0"
            width="100"
            height="52"
            rx="4"
            fill="var(--bg-2)"
            stroke="var(--accent-border)"
          />
          <text
            x="10"
            y="17"
            fontFamily="var(--font-mono)"
            fontSize="10.5"
            fill="var(--text)"
            fontWeight="600"
          >
            anchored
          </text>
          <g transform="translate(70 7)">
            <rect
              x="0"
              y="0"
              width="22"
              height="13"
              rx="2.5"
              fill="var(--accent-bg)"
              stroke="var(--accent-border)"
            />
            <text
              x="11"
              y="10"
              fontFamily="var(--font-mono)"
              fontSize="8.5"
              fill="var(--accent)"
              fontWeight="700"
              textAnchor="middle"
              letterSpacing="0.6"
            >
              OSS
            </text>
          </g>
          <text
            x="10"
            y="32"
            fontFamily="var(--font-mono)"
            fontSize="9.5"
            fill="var(--text-muted)"
          >
            your infra
          </text>
          <text
            x="10"
            y="45"
            fontFamily="var(--font-mono)"
            fontSize="9.5"
            fill="var(--text-dim)"
          >
            postgres
          </text>
        </g>
      </g>

      {/* POLICY: gate */}
      <g>
        <rect
          x={COL_LEFT + 10}
          y={GATE_Y - 16}
          width={W - COL_LEFT - 24}
          height="32"
          rx="4"
          fill="var(--bg-2)"
          stroke="var(--border-strong)"
        />
        <text
          x={(W + COL_LEFT) / 2 + 5}
          y={GATE_Y + 4}
          fontFamily="var(--font-mono)"
          fontSize="11"
          fill="var(--text-muted)"
          textAnchor="middle"
          letterSpacing="0.5"
        >
          policy gate · sanitise · audit
        </text>
      </g>

      {/* LOCAL: three devs */}
      {DEV_X.map((x, i) => (
        <g key={i}>
          <IsoCube cx={x} cy={LOCAL_Y + 16} size={16} dim />
          <text
            x={x}
            y={LOCAL_Y + 48}
            fontFamily="var(--font-mono)"
            fontSize="11"
            fontWeight="600"
            fill="var(--text)"
            textAnchor="middle"
          >
            dev {i + 1}
          </text>
          <text
            x={x}
            y={LOCAL_Y + 62}
            fontFamily="var(--font-mono)"
            fontSize="9.5"
            fill="var(--text-dim)"
            textAnchor="middle"
          >
            anchored · mcp
          </text>
        </g>
      ))}

      {/* Arrows */}
      <g
        stroke="var(--accent)"
        strokeWidth="1.4"
        strokeDasharray="3 4"
        fill="none"
        opacity="0.85"
      >
        {DEV_X.map((x, i) => (
          <line key={`a${i}`} x1={x} y1={LOCAL_Y - 4} x2={x} y2={GATE_Y + 22} />
        ))}
        <line
          x1={W / 2}
          y1={GATE_Y - 20}
          x2={W / 2}
          y2={REMOTE_Y + 30}
        />
      </g>
      {DEV_X.map((x, i) => (
        <polygon
          key={`h${i}`}
          points={`${x},${GATE_Y + 22} ${x - 5},${GATE_Y + 30} ${x + 5},${GATE_Y + 30}`}
          fill="var(--accent)"
        />
      ))}
      <polygon
        points={`${W / 2},${REMOTE_Y + 30} ${W / 2 - 5},${REMOTE_Y + 38} ${W / 2 + 5},${REMOTE_Y + 38}`}
        fill="var(--accent)"
      />

      {/* Caption */}
      <g transform={`translate(${W / 2 - 105} ${(GATE_Y + LOCAL_Y) / 2 - 14})`}>
        <rect
          x="0"
          y="0"
          width="210"
          height="28"
          rx="14"
          fill="var(--bg-1)"
          stroke="var(--accent-border)"
        />
        <text
          x="105"
          y="13"
          fontFamily="var(--font-mono)"
          fontSize="10"
          fill="var(--accent)"
          textAnchor="middle"
          letterSpacing="0.4"
        >
          shared knowledge ↑
        </text>
        <text
          x="105"
          y="23"
          fontFamily="var(--font-mono)"
          fontSize="9"
          fill="var(--text-dim)"
          textAnchor="middle"
        >
          decisions · conventions · learnings
        </text>
      </g>

      <g transform={`translate(${W / 2} ${H - 8})`}>
        <text
          fontFamily="var(--font-mono)"
          fontSize="9.5"
          fill="var(--text-ghost)"
          textAnchor="middle"
          letterSpacing="0.4"
        >
          machine-local context · stays put · never leaves dev's box
        </text>
      </g>
    </svg>
  );
}

// ─── IsoCube ────────────────────────────────────────────────
function IsoCube({
  cx,
  cy,
  size = 80,
  dim = false,
  accent = true,
}: {
  cx: number;
  cy: number;
  size?: number;
  dim?: boolean;
  accent?: boolean;
}) {
  const s = size;
  const top = `${cx},${cy - s} ${cx + s * 0.866},${cy - s * 0.5} ${cx},${cy} ${cx - s * 0.866},${cy - s * 0.5}`;
  const left = `${cx - s * 0.866},${cy - s * 0.5} ${cx},${cy} ${cx},${cy + s} ${cx - s * 0.866},${cy + s * 0.5}`;
  const right = `${cx},${cy} ${cx + s * 0.866},${cy - s * 0.5} ${cx + s * 0.866},${cy + s * 0.5} ${cx},${cy + s}`;
  return (
    <g opacity={dim ? 0.55 : 1}>
      <polygon points={top} fill="var(--text)" opacity={dim ? 0.5 : 0.9} />
      <polygon points={left} fill="var(--text)" opacity={dim ? 0.32 : 0.55} />
      <polygon
        points={right}
        fill={accent ? "var(--accent)" : "var(--text)"}
        opacity={dim ? 0.7 : 1}
      />
      <polyline
        points={`${cx - s * 0.866},${cy - s * 0.5} ${cx},${cy} ${cx + s * 0.866},${cy - s * 0.5}`}
        stroke="var(--bg)"
        strokeWidth="1"
        fill="none"
        opacity="0.6"
      />
      <line
        x1={cx}
        y1={cy}
        x2={cx}
        y2={cy + s}
        stroke="var(--bg)"
        strokeWidth="1"
        opacity="0.6"
      />
    </g>
  );
}
