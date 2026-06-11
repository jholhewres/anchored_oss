import { useCallback, useEffect, useMemo, useState } from "react";
import { api, TaskThread } from "@/lib/api";
import { Badge, Btn, Empty, Input } from "@/ds/components";
import { I } from "@/ds/icons";

// MyTasksPage is the personal kanban: a READ-MOSTLY view of the caller's own
// task threads, synced from their anchored client. Cards are auto-populated
// (branch inference + `anchored task`) — this page deliberately has no CRUD:
// the source of truth for task management stays in Jira/Trello; what anchored
// adds is WHERE in the code the task lived and what was learned along the way.

type Tone = "ok" | "warn" | "neutral";

interface Column {
  status: string;
  label: string;
  tone: Tone;
  hint: string;
}

const columns: Column[] = [
  { status: "active", label: "Active", tone: "ok", hint: "Threads touched by a recent session" },
  { status: "paused", label: "Paused", tone: "warn", hint: "Parked — resumes on next touch" },
  { status: "done", label: "Done", tone: "neutral", hint: "Finished or cancelled threads" },
];

const toneColor: Record<Tone, string> = {
  ok: "var(--ok)",
  warn: "var(--warn)",
  neutral: "var(--text-dim)",
};

// absoluteTime renders the full timestamp for title attributes, falling back
// to the raw string when the date doesn't parse (avoids "Invalid Date").
function absoluteTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// timeAgo renders a compact relative timestamp; the absolute value goes in
// the title attribute so hover always shows the precise moment.
function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "—";
  const s = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${d}d ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.floor(mo / 12)}y ago`;
}

// refDomain shortens an external link to its host for display — the full URL
// stays in href/title.
function refDomain(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function isHttpUrl(s: string): boolean {
  return /^https?:\/\//.test(s);
}

function matchesQuery(t: TaskThread, q: string): boolean {
  if (!q) return true;
  const needle = q.toLowerCase();
  if (t.task_key.toLowerCase().includes(needle)) return true;
  if (t.external_ref?.toLowerCase().includes(needle)) return true;
  if (t.projects?.some((p) => p.toLowerCase().includes(needle))) return true;
  if (t.journal?.some((n) => n.toLowerCase().includes(needle))) return true;
  return false;
}

// ── Card ────────────────────────────────────────────────────────────────────

const journalPreviewCount = 3;

function TaskCard({ thread, tone }: { thread: TaskThread; tone: Tone }) {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const [hover, setHover] = useState(false);

  const journal = thread.journal ?? [];
  const visibleNotes = expanded ? journal : journal.slice(0, journalPreviewCount);
  const hiddenCount = Math.max(0, journal.length - journalPreviewCount);
  const cancelled = thread.status === "cancelled";
  const accent = cancelled ? "var(--err)" : toneColor[tone];

  const copyKey = async () => {
    try {
      await navigator.clipboard.writeText(thread.task_key);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      // Clipboard may be unavailable (http, permissions) — the key is visible
      // and selectable anyway, so failing silently is fine.
    }
  };

  return (
    <div
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        position: "relative",
        background: "var(--bg-2)",
        border: `1px solid ${hover ? "var(--border-strong)" : "var(--border)"}`,
        borderRadius: "var(--radius-lg)",
        padding: "12px 14px 12px 16px",
        transition: "border-color .12s, transform .12s",
        transform: hover ? "translateY(-1px)" : "none",
      }}
    >
      {/* status accent strip */}
      <span
        style={{
          position: "absolute",
          left: 0,
          top: 10,
          bottom: 10,
          width: 2,
          borderRadius: 2,
          background: accent,
          opacity: 0.85,
        }}
      />

      {/* key row */}
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 13,
            fontWeight: 600,
            letterSpacing: 0.2,
            color: cancelled ? "var(--text-muted)" : "var(--text)",
            textDecoration: cancelled ? "line-through" : "none",
          }}
        >
          {thread.task_key}
        </span>
        <button
          type="button"
          onClick={copyKey}
          aria-label={copied ? "Task key copied" : `Copy ${thread.task_key}`}
          title="Copy task key"
          style={{
            border: 0,
            background: "transparent",
            color: copied ? "var(--ok)" : "var(--text-dim)",
            cursor: "pointer",
            display: "inline-flex",
            alignItems: "center",
            padding: 2,
            opacity: hover || copied ? 1 : 0,
            transition: "opacity .12s, color .12s",
          }}
        >
          {copied ? <I.check size={12} /> : <I.copy size={12} />}
        </button>
        <span style={{ flex: 1 }} />
        {cancelled && <Badge tone="err">cancelled</Badge>}
        <span
          title={absoluteTime(thread.updated_at)}
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 11,
            color: "var(--text-dim)",
            whiteSpace: "nowrap",
            display: "inline-flex",
            alignItems: "center",
            gap: 4,
          }}
        >
          <I.clock size={11} />
          {timeAgo(thread.updated_at)}
        </span>
      </div>

      {/* external ref */}
      {thread.external_ref && isHttpUrl(thread.external_ref) && (
        <div style={{ marginTop: 6 }}>
          <a
            href={thread.external_ref}
            target="_blank"
            rel="noreferrer"
            title={thread.external_ref}
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 5,
              fontSize: 12,
              color: "var(--text-muted)",
            }}
          >
            <I.external size={11} />
            {refDomain(thread.external_ref)}
          </a>
        </div>
      )}

      {/* project chips */}
      {thread.projects && thread.projects.length > 0 && (
        <div style={{ marginTop: 8, display: "flex", flexWrap: "wrap", gap: 6 }}>
          {thread.projects.map((p) => (
            <Badge key={p} tone="neutral" icon={<I.folder />}>
              {p}
            </Badge>
          ))}
        </div>
      )}

      {/* SAFETY: journal is user-supplied text — render as text only, never
          via dangerouslySetInnerHTML or markdown. */}
      {journal.length > 0 && (
        <div style={{ marginTop: 10 }}>
          <ul
            style={{
              margin: 0,
              padding: 0,
              listStyle: "none",
              display: "flex",
              flexDirection: "column",
              gap: 5,
            }}
          >
            {visibleNotes.map((n, i) => (
              <li
                key={i}
                style={{
                  display: "flex",
                  gap: 8,
                  fontSize: 12,
                  lineHeight: 1.5,
                  color: i === 0 && !expanded ? "var(--text-muted)" : "var(--text-dim)",
                }}
              >
                <span
                  aria-hidden
                  style={{
                    flex: "none",
                    marginTop: 7,
                    width: 4,
                    height: 4,
                    borderRadius: 2,
                    background: "var(--text-ghost)",
                  }}
                />
                <span style={{ overflowWrap: "anywhere" }}>{n}</span>
              </li>
            ))}
          </ul>
          {hiddenCount > 0 && (
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              style={{
                marginTop: 6,
                border: 0,
                background: "transparent",
                color: "var(--text-dim)",
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                cursor: "pointer",
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
                padding: 0,
              }}
            >
              {expanded ? <I.chevU size={11} /> : <I.chevD size={11} />}
              {expanded ? "show less" : `+${hiddenCount} more note${hiddenCount > 1 ? "s" : ""}`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// ── Skeleton (loading) ──────────────────────────────────────────────────────

function SkeletonCard({ delay }: { delay: number }) {
  const bar = (w: string, h = 10) => (
    <div
      style={{
        width: w,
        height: h,
        borderRadius: 4,
        background: "var(--bg-3)",
        animation: "a-blink 1.4s ease-in-out infinite",
        animationDelay: `${delay}ms`,
      }}
    />
  );
  return (
    <div
      style={{
        background: "var(--bg-2)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)",
        padding: 14,
        display: "flex",
        flexDirection: "column",
        gap: 10,
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between" }}>
        {bar("38%", 12)}
        {bar("18%")}
      </div>
      {bar("55%")}
      {bar("80%")}
    </div>
  );
}

// ── Page ────────────────────────────────────────────────────────────────────

export function MyTasksPage() {
  const [threads, setThreads] = useState<TaskThread[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");

  const load = useCallback(async (asRefresh: boolean) => {
    if (asRefresh) setRefreshing(true);
    try {
      const r = await api.listMyTaskThreads();
      setThreads(r.threads ?? []);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load task threads");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    load(false);
  }, [load]);

  const filtered = useMemo(
    () => threads.filter((t) => matchesQuery(t, query.trim())),
    [threads, query],
  );

  const byStatus = (s: string) =>
    filtered.filter((t) => (s === "done" ? t.status === "done" || t.status === "cancelled" : t.status === s));

  const board = (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
        gap: 16,
        alignItems: "start",
      }}
    >
      {columns.map((col) => {
        const items = byStatus(col.status);
        return (
          <section
            key={col.status}
            aria-label={`${col.label} tasks`}
            style={{
              background: "var(--bg-1)",
              border: "1px solid var(--border)",
              borderRadius: "var(--radius-lg)",
              padding: 10,
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "4px 6px 10px",
              }}
              title={col.hint}
            >
              <span
                style={{
                  width: 7,
                  height: 7,
                  borderRadius: 4,
                  background: toneColor[col.tone],
                  boxShadow: `0 0 0 3px color-mix(in srgb, ${toneColor[col.tone]} 18%, transparent)`,
                  flex: "none",
                }}
              />
              <span style={{ fontSize: 13, fontWeight: 600, letterSpacing: -0.1 }}>{col.label}</span>
              <Badge tone={col.tone}>{items.length}</Badge>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
              {loading
                ? [0, 1].map((i) => <SkeletonCard key={i} delay={i * 180} />)
                : items.map((t) => <TaskCard key={t.task_key} thread={t} tone={col.tone} />)}
              {!loading && items.length === 0 && (
                <div
                  style={{
                    border: "1px dashed var(--border-strong)",
                    borderRadius: "var(--radius)",
                    padding: "18px 12px",
                    textAlign: "center",
                    fontSize: 12,
                    color: "var(--text-ghost)",
                    fontFamily: "var(--font-mono)",
                  }}
                >
                  {query ? "no matches" : "nothing here"}
                </div>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }}>
      {/* header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <div style={{ marginRight: "auto" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
            <h1 style={{ margin: 0, fontSize: 18, fontWeight: 600, letterSpacing: -0.3 }}>My tasks</h1>
            {!loading && (
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
                {threads.length} thread{threads.length === 1 ? "" : "s"}
              </span>
            )}
          </div>
          <div style={{ marginTop: 4, fontSize: 12.5, color: "var(--text-muted)" }}>
            Synced from your anchored client — where each ticket lived across projects.
          </div>
        </div>
        <Input
          icon={<I.search />}
          size="sm"
          placeholder="Filter by key, project, note…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-label="Filter task threads"
          style={{ width: 230 }}
        />
        <Btn
          size="sm"
          icon={<I.refresh />}
          onClick={() => load(true)}
          disabled={refreshing}
          aria-label="Refresh task threads"
        >
          {refreshing ? "Refreshing…" : "Refresh"}
        </Btn>
      </div>

      {/* error — shown ALONGSIDE the board on purpose: a failed refresh keeps
          the stale threads visible instead of blanking the page. */}
      {error && !loading && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "10px 14px",
            background: "var(--err-bg)",
            border: "1px solid color-mix(in srgb, var(--err) 25%, transparent)",
            borderRadius: "var(--radius)",
            color: "var(--err)",
            fontSize: 13,
          }}
        >
          <I.alert size={14} />
          <span style={{ flex: 1 }}>{error}</span>
          <Btn size="sm" variant="outline" onClick={() => load(true)}>
            Retry
          </Btn>
        </div>
      )}

      {/* body */}
      {!loading && !error && threads.length === 0 ? (
        <Empty
          icon={<I.check />}
          title="No task threads yet"
          body={
            <>
              Work on a branch named after a ticket (<code>feature/PROJ-123-…</code>) or run{" "}
              <code>anchored task start PROJ-123</code> — threads sync here automatically.
            </>
          }
        />
      ) : (
        board
      )}
    </div>
  );
}
