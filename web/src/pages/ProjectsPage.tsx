import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import { PageHeader } from "@/components/PageHeader";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";

export function ProjectsPage() {
  const navigate = useNavigate();
  const { data, isLoading, error } = useQuery({
    queryKey: ["projects"],
    queryFn: () => api.getProjects(),
  });

  return (
    <>
      <PageHeader
        title="Projects"
        description="Projects you have team access to. Soft-deleted projects are excluded."
      />
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="space-y-3 p-6">
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ) : error ? (
            <p className="p-6 text-sm text-destructive">Failed to load projects.</p>
          ) : !data || data.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No projects yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Remote key</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((p) => (
                  <TableRow
                    key={p.id}
                    onClick={() => navigate(`/projects/${p.id}`)}
                    className="cursor-pointer"
                  >
                    <TableCell className="font-medium">{p.name}</TableCell>
                    <TableCell>
                      <code className="text-xs text-muted-foreground">{p.slug}</code>
                    </TableCell>
                    <TableCell>
                      <code className="text-xs text-muted-foreground">{p.remote_key}</code>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(p.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </>
  );
}
