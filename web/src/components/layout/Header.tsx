import { useAuth } from "@/lib/auth";
import { useLocation } from "react-router-dom";

const routeTitles: Record<string, string> = {
  "/": "Overview",
  "/projects": "Projects",
  "/accounts": "Accounts",
  "/teams": "Teams",
  "/api-keys": "API keys",
  "/audit": "Audit log",
  "/health": "Health",
};

export function Header() {
  const { me } = useAuth();
  const location = useLocation();

  if (!me) return null;

  const title =
    routeTitles[location.pathname] ||
    (location.pathname.startsWith("/projects/") ? "Project" : null) ||
    (location.pathname.startsWith("/teams/") ? "Team" : null) ||
    "Dashboard";

  return (
    <header
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "14px 36px",
        borderBottom: "1px solid var(--border)",
        background: "var(--bg)",
        minHeight: 60,
      }}
    >
      <div>
        <h1
          style={{
            fontSize: 22,
            fontWeight: 500,
            letterSpacing: -0.5,
            margin: 0,
            lineHeight: 1.1,
          }}
        >
          {title}
        </h1>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 12,
            color: "var(--text-dim)",
          }}
        >
          {me.email}
        </span>
      </div>
    </header>
  );
}
