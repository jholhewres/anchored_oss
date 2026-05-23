import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import type { AuditEntry, AuditFilters } from "@/lib/types";
import { formatNumber } from "@/lib/utils";

const PAGE_SIZE = 50;
const actionOptions = [
  "",
  "sync.push.accepted",
  "sync.push.rejected",
  "sync.tombstone.accepted",
  "sync.project.created",
];

export function AuditPage() {
  const [filters, setFilters] = useState<AuditFilters>({ limit: PAGE_SIZE, offset: 0 });
  const projectsQuery = useQuery({ queryKey: ["projects"], queryFn: () => api.getProjects() });
  const accountsQuery = useQuery({ queryKey: ["accounts"], queryFn: () => api.getAccounts() });

  const { data, isLoading } = useQuery({
    queryKey: ["audit", filters],
    queryFn: () => api.getAudit(filters),
    placeholderData: (prev) => prev,
  });

  const total = data?.total ?? 0;
  const offset = filters.offset ?? 0;
  const limit = data?.limit ?? PAGE_SIZE;
  const showingTo = offset + (data?.entries.length ?? 0);

  function setFilter<K extends keyof AuditFilters>(key: K, value: AuditFilters[K]) {
    setFilters((prev) => ({ ...prev, [key]: value, offset: 0 }));
  }

  return (
    <>
      <PageHeader
        title="Audit"
        description={`${formatNumber(total)} entries · showing ${total === 0 ? 0 : offset + 1}–${showingTo}`}
      />

      <Card className="mb-4">
        <CardContent className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-5">
          <div className="space-y-2">
            <Label className="text-xs">Project</Label>
            <Select
              value={filters.project ?? ""}
              onValueChange={(v) => setFilter("project", v || undefined)}
            >
              <SelectTrigger>
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__any">All</SelectItem>
                {(projectsQuery.data ?? []).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label className="text-xs">Actor</Label>
            <Select
              value={filters.actor ?? ""}
              onValueChange={(v) => setFilter("actor", v || undefined)}
            >
              <SelectTrigger>
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__any">All</SelectItem>
                {(accountsQuery.data ?? []).map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.display_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label className="text-xs">Action</Label>
            <Select
              value={filters.action ?? ""}
              onValueChange={(v) => setFilter("action", v || undefined)}
            >
              <SelectTrigger>
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                {actionOptions.map((a) => (
                  <SelectItem key={a || "__any"} value={a || "__any"}>
                    {a || "All"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label className="text-xs">From</Label>
            <Input
              type="datetime-local"
              value={filters.from ?? ""}
              onChange={(e) => setFilter("from", e.target.value || undefined)}
            />
          </div>
          <div className="space-y-2">
            <Label className="text-xs">To</Label>
            <Input
              type="datetime-local"
              value={filters.to ?? ""}
              onChange={(e) => setFilter("to", e.target.value || undefined)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {isLoading && !data ? (
            <div className="space-y-3 p-6">
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ) : !data || data.entries.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No audit entries.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Timestamp</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Target</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.entries.map((e) => (
                  <AuditRow key={e.id} entry={e} />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <div className="mt-4 flex justify-end gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={offset === 0}
          onClick={() => setFilters((f) => ({ ...f, offset: Math.max(0, offset - limit) }))}
        >
          Previous
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={showingTo >= total}
          onClick={() => setFilters((f) => ({ ...f, offset: offset + limit }))}
        >
          Next
        </Button>
      </div>
    </>
  );
}

function AuditRow({ entry }: { entry: AuditEntry }) {
  const [open, setOpen] = useState(false);
  const meta = entry.metadata;
  const hasMeta = meta != null && typeof meta === "object" && Object.keys(meta as object).length > 0;
  return (
    <>
      <TableRow
        onClick={() => hasMeta && setOpen((v) => !v)}
        className={hasMeta ? "cursor-pointer" : ""}
      >
        <TableCell className="text-xs text-muted-foreground">
          {new Date(entry.created_at).toLocaleString()}
        </TableCell>
        <TableCell>
          <code className="text-xs">{entry.action}</code>
        </TableCell>
        <TableCell className="text-xs text-muted-foreground">
          {entry.actor_id ? entry.actor_id.slice(0, 8) : "—"}
        </TableCell>
        <TableCell className="text-xs text-muted-foreground">
          {entry.target_type ?? "—"}
          {entry.target_id && (
            <span className="ml-2">
              <code>{entry.target_id.slice(0, 8)}</code>
            </span>
          )}
        </TableCell>
      </TableRow>
      {open && hasMeta && (
        <TableRow>
          <TableCell colSpan={4} className="bg-muted/30">
            <pre className="overflow-auto text-xs">{JSON.stringify(meta, null, 2)}</pre>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
