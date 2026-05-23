import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { Trash2 } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatNumber, formatRelativeTime, truncate } from "@/lib/utils";

const PAGE_SIZE = 20;

export function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const { me } = useAuth();
  const [offset, setOffset] = useState(0);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const projectQuery = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id!),
    enabled: Boolean(id),
  });

  const memoriesQuery = useQuery({
    queryKey: ["project-memories", id, offset],
    queryFn: () => api.getProjectMemories(id!, PAGE_SIZE, offset),
    enabled: Boolean(id),
    placeholderData: (prev) => prev,
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteProject(id!),
    onSuccess: () => {
      toast.push({ title: "Project deleted", variant: "success" });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate("/projects");
    },
    onError: (err) => {
      toast.push({
        title: "Delete failed",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    },
  });

  const project = projectQuery.data;
  const memories = memoriesQuery.data;
  const total = memories?.total ?? 0;
  const pageEnd = offset + (memories?.memories.length ?? 0);
  const canPrev = offset > 0;
  const canNext = pageEnd < total;

  return (
    <>
      <PageHeader
        title={project?.name ?? "Project"}
        description={project?.remote_key}
        actions={
          me?.scope === "admin" && (
            <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="mr-2 h-4 w-4" /> Delete
            </Button>
          )
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2 text-sm sm:grid-cols-2">
          {projectQuery.isLoading ? (
            <Skeleton className="h-24 w-full" />
          ) : project ? (
            <>
              <DetailRow label="ID" value={<code className="text-xs">{project.id}</code>} />
              <DetailRow label="Slug" value={<code className="text-xs">{project.slug}</code>} />
              <DetailRow label="Created by" value={<code className="text-xs">{project.created_by}</code>} />
              <DetailRow label="Created" value={formatRelativeTime(project.created_at)} />
            </>
          ) : (
            <p className="text-sm text-destructive">Project not found.</p>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Memories</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {formatNumber(total)} total · showing {offset + 1}–{pageEnd}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={!canPrev}
              onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            >
              Previous
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!canNext}
              onClick={() => setOffset(offset + PAGE_SIZE)}
            >
              Next
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {memoriesQuery.isLoading ? (
            <div className="space-y-3 p-6">
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ) : !memories || memories.memories.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No memories.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Category</TableHead>
                  <TableHead>Content</TableHead>
                  <TableHead>Author</TableHead>
                  <TableHead>Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {memories.memories.map((m) => (
                  <TableRow key={m.id}>
                    <TableCell>
                      <Badge variant="secondary">{m.category}</Badge>
                    </TableCell>
                    <TableCell className="max-w-md">
                      <span className="text-sm">{truncate(m.content, 120)}</span>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{m.author_name ?? "—"}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(m.updated_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete project?</DialogTitle>
            <DialogDescription>
              The project will be soft-deleted. Memories remain in the database but are no
              longer visible. This action can be reversed by an operator with database access.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                deleteMutation.mutate();
                setDeleteOpen(false);
              }}
            >
              Delete project
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-border/40 py-2 last:border-0">
      <span className="text-muted-foreground">{label}</span>
      <span>{value}</span>
    </div>
  );
}
