import { useQuery } from "@tanstack/react-query";
import { Activity, Database, FolderGit2, KeyRound, ScrollText, UsersRound, Building2, Layers } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import { formatNumber, formatRelativeTime } from "@/lib/utils";
import { useAuth } from "@/lib/auth";

const cards = [
  { key: "accounts" as const, label: "Accounts", icon: UsersRound },
  { key: "organizations" as const, label: "Organizations", icon: Building2 },
  { key: "teams" as const, label: "Teams", icon: Layers },
  { key: "projects" as const, label: "Projects", icon: FolderGit2 },
  { key: "memories_live" as const, label: "Live memories", icon: Database },
  { key: "keys_active" as const, label: "Active keys", icon: KeyRound },
  { key: "audit_entries_24h" as const, label: "Audit (24h)", icon: ScrollText },
];

export function OverviewPage() {
  const { me } = useAuth();
  const isAdmin = me?.scope === "admin";
  const { data, isLoading, error } = useQuery({
    queryKey: ["stats"],
    queryFn: () => api.getStats(),
    refetchInterval: 30_000,
    enabled: isAdmin,
  });

  if (!isAdmin) {
    return (
      <>
        <PageHeader title="Overview" />
        <Card>
          <CardHeader>
            <CardTitle>Admin scope required</CardTitle>
            <CardDescription>
              The overview dashboard aggregates org-level data and is restricted to admin keys.
            </CardDescription>
          </CardHeader>
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader title="Overview" description="Real-time view of your organization." />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map(({ key, label, icon: Icon }) => (
          <Card key={key}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
              <Icon className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-7 w-20" />
              ) : (
                <div className="text-2xl font-semibold">
                  {data ? formatNumber(data[key]) : "—"}
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Activity className="h-4 w-4" /> Recent push activity (last 24h)
          </CardTitle>
          <CardDescription>Top projects by accepted memory pushes.</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : error ? (
            <p className="text-sm text-destructive">Failed to load stats.</p>
          ) : data && data.recent_pushes.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Project</TableHead>
                  <TableHead className="text-right">Pushes</TableHead>
                  <TableHead className="text-right">Last push</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.recent_pushes.map((p) => (
                  <TableRow key={p.project_id}>
                    <TableCell className="font-medium">{p.project_name}</TableCell>
                    <TableCell className="text-right">{formatNumber(p.count)}</TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      {formatRelativeTime(p.last_push)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No push activity in the last 24 hours.
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}
