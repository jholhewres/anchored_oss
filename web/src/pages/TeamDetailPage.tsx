import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { Plus, Trash2 } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatRelativeTime } from "@/lib/utils";

export function TeamDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const toast = useToast();
  const { me } = useAuth();
  const isAdmin = me?.scope === "admin";
  const [addOpen, setAddOpen] = useState(false);
  const [selected, setSelected] = useState<string | undefined>();

  const teamQuery = useQuery({
    queryKey: ["team", id],
    queryFn: () => api.getTeam(id!),
    enabled: Boolean(id),
  });
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api.getAccounts(),
    enabled: isAdmin,
  });

  const team = teamQuery.data;

  const available = useMemo(() => {
    if (!team || !accountsQuery.data) return [];
    const memberIds = new Set(team.members.map((m) => m.account_id));
    return accountsQuery.data.filter((a) => !memberIds.has(a.id));
  }, [team, accountsQuery.data]);

  const addMember = useMutation({
    mutationFn: (accountId: string) => api.addTeamMember(id!, accountId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["team", id] });
      setAddOpen(false);
      setSelected(undefined);
      toast.push({ title: "Member added", variant: "success" });
    },
    onError: (err) =>
      toast.push({
        title: "Add member failed",
        description: err instanceof Error ? err.message : "",
        variant: "error",
      }),
  });

  const removeMember = useMutation({
    mutationFn: (accountId: string) => api.removeTeamMember(id!, accountId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["team", id] });
      toast.push({ title: "Member removed", variant: "success" });
    },
    onError: (err) =>
      toast.push({
        title: "Remove member failed",
        description: err instanceof Error ? err.message : "",
        variant: "error",
      }),
  });

  if (teamQuery.isLoading) {
    return (
      <>
        <PageHeader title="Team" />
        <Skeleton className="h-32 w-full" />
      </>
    );
  }
  if (!team) {
    return (
      <>
        <PageHeader title="Team" description="Not found." />
      </>
    );
  }

  return (
    <>
      <PageHeader title={team.name} description={`Slug: ${team.slug}`} />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Members ({team.members.length})</CardTitle>
          {isAdmin && (
            <Dialog open={addOpen} onOpenChange={setAddOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="mr-2 h-4 w-4" /> Add member
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add member</DialogTitle>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label>Account</Label>
                    <Select value={selected} onValueChange={setSelected}>
                      <SelectTrigger>
                        <SelectValue placeholder="Pick an account" />
                      </SelectTrigger>
                      <SelectContent>
                        {available.map((a) => (
                          <SelectItem key={a.id} value={a.id}>
                            {a.display_name} · {a.email}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setAddOpen(false)}>
                    Cancel
                  </Button>
                  <Button
                    onClick={() => selected && addMember.mutate(selected)}
                    disabled={!selected || addMember.isPending}
                  >
                    Add
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          )}
        </CardHeader>
        <CardContent className="p-0">
          {team.members.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No members yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Email</TableHead>
                  <TableHead>Display name</TableHead>
                  <TableHead>Added</TableHead>
                  {isAdmin && <TableHead className="w-px" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {team.members.map((m) => (
                  <TableRow key={m.account_id}>
                    <TableCell className="font-medium">{m.email}</TableCell>
                    <TableCell>{m.display_name}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(m.added_at)}
                    </TableCell>
                    {isAdmin && (
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => removeMember.mutate(m.account_id)}
                          aria-label="Remove"
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Project access</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {team.project_grants.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              This team has no project grants yet.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Project</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Role</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {team.project_grants.map((g) => (
                  <TableRow key={g.project_id}>
                    <TableCell className="font-medium">{g.project_name}</TableCell>
                    <TableCell>
                      <code className="text-xs text-muted-foreground">{g.project_slug}</code>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{g.role}</Badge>
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
