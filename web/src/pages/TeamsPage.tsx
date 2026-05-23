import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { Plus } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatRelativeTime } from "@/lib/utils";

export function TeamsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const { me } = useAuth();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["teams"],
    queryFn: () => api.getTeams(),
  });

  const create = useMutation({
    mutationFn: () => api.createTeam(name, slug),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["teams"] });
      setOpen(false);
      setName("");
      setSlug("");
      toast.push({ title: "Team created", variant: "success" });
    },
    onError: (err) => {
      toast.push({
        title: "Create team failed",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    create.mutate();
  }

  return (
    <>
      <PageHeader
        title="Teams"
        description="Groups that bundle members and project access grants."
        actions={
          me?.scope === "admin" && (
            <Dialog open={open} onOpenChange={setOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="mr-2 h-4 w-4" /> Create team
                </Button>
              </DialogTrigger>
              <DialogContent>
                <form onSubmit={onSubmit}>
                  <DialogHeader>
                    <DialogTitle>Create team</DialogTitle>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <Label htmlFor="name">Name</Label>
                      <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="slug">Slug</Label>
                      <Input
                        id="slug"
                        value={slug}
                        onChange={(e) => setSlug(e.target.value)}
                        placeholder="lowercase-kebab"
                        pattern="[a-z0-9][a-z0-9-]{0,63}"
                        required
                      />
                    </div>
                  </div>
                  <DialogFooter>
                    <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                      Cancel
                    </Button>
                    <Button type="submit" disabled={create.isPending}>
                      Create
                    </Button>
                  </DialogFooter>
                </form>
              </DialogContent>
            </Dialog>
          )
        }
      />
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="space-y-3 p-6">
              <Skeleton className="h-9 w-full" />
            </div>
          ) : !data || data.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No teams yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((t) => (
                  <TableRow
                    key={t.id}
                    onClick={() => navigate(`/teams/${t.id}`)}
                    className="cursor-pointer"
                  >
                    <TableCell className="font-medium">{t.name}</TableCell>
                    <TableCell>
                      <code className="text-xs text-muted-foreground">{t.slug}</code>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(t.created_at)}
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
