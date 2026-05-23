import { NavLink } from "react-router-dom";
import {
  Anchor,
  LayoutDashboard,
  FolderGit2,
  Users,
  UsersRound,
  KeyRound,
  ScrollText,
  Activity,
  ExternalLink,
} from "lucide-react";

import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

interface NavItem {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
  adminOnly?: boolean;
  end?: boolean;
}

const items: NavItem[] = [
  { to: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { to: "/projects", label: "Projects", icon: FolderGit2 },
  { to: "/accounts", label: "Accounts", icon: Users, adminOnly: true },
  { to: "/teams", label: "Teams", icon: UsersRound },
  { to: "/api-keys", label: "API keys", icon: KeyRound, adminOnly: true },
  { to: "/audit", label: "Audit", icon: ScrollText, adminOnly: true },
];

export function Sidebar() {
  const { me } = useAuth();
  const isAdmin = me?.scope === "admin";
  return (
    <aside className="flex w-64 shrink-0 flex-col border-r bg-card/40">
      <div className="flex h-14 items-center gap-2 border-b px-5">
        <Anchor className="h-5 w-5 text-primary" />
        <span className="text-sm font-semibold">anchored-oss</span>
        <span className="ml-auto rounded-md bg-secondary px-2 py-0.5 text-[10px] uppercase tracking-wider text-muted-foreground">
          default
        </span>
      </div>
      <nav className="flex-1 space-y-0.5 p-3">
        {items.map((item) => {
          if (item.adminOnly && !isAdmin) return null;
          const Icon = item.icon;
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                  isActive
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                )
              }
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          );
        })}
      </nav>
      <div className="space-y-0.5 border-t p-3">
        <p className="px-3 pb-1 text-[10px] uppercase tracking-wider text-muted-foreground">
          System
        </p>
        <NavLink
          to="/health"
          className={({ isActive }) =>
            cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
              isActive
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
            )
          }
        >
          <Activity className="h-4 w-4" />
          Health
        </NavLink>
        <a
          href="https://github.com/jholhewres/anchored_oss"
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground"
        >
          <ExternalLink className="h-4 w-4" />
          Docs
        </a>
      </div>
    </aside>
  );
}
