import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Plus, ShieldAlert } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";
import type { APIKey, APIKeyMintResponse, Scope } from "@/lib/types";

const scopeOptions: { value: Scope; label: string }[] = [
  { value: "admin", label: "admin" },
  { value: "sync", label: "sync" },
  { value: "readonly", label: "readonly" },
];

const expiryOptions = [
  { value: "", label: "Never" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
];

export function APIKeysPage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [scope, setScope] = useState<Scope>("sync");
  const [accountId, setAccountId] = useState<string>("");
  const [expiresIn, setExpiresIn] = useState<string>("");
  const [minted, setMinted] = useState<APIKeyMintResponse | null>(null);

  const keysQuery = useQuery({ queryKey: ["api-keys"], queryFn: () => api.getAPIKeys() });
  const accountsQuery = useQuery({ queryKey: ["accounts"], queryFn: () => api.getAccounts() });

  const create = useMutation({
    mutationFn: () => api.createAPIKey(name, scope, accountId, expiresIn),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setMinted(resp);
      setOpen(false);
    },
    onError: (err) =>
      toast.push({
        title: "Mint failed",
        description: err instanceof Error ? err.message : "",
        variant: "error",
      }),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeAPIKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      toast.push({ title: "Key revoked", variant: "success" });
    },
    onError: (err) =>
      toast.push({
        title: "Revoke failed",
        description: err instanceof Error ? err.message : "",
        variant: "error",
      }),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!name || !scope || !accountId) return;
    create.mutate();
  }

  function resetForm() {
    setName("");
    setScope("sync");
    setAccountId("");
    setExpiresIn("");
  }

  return (
    <>
      <PageHeader
        title="API keys"
        description="Bearer tokens used by clients and operators."
        actions={
          <Dialog
            open={open}
            onOpenChange={(o) => {
              setOpen(o);
              if (!o) resetForm();
            }}
          >
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" /> Mint key
              </Button>
            </DialogTrigger>
            <DialogContent>
              <form onSubmit={onSubmit}>
                <DialogHeader>
                  <DialogTitle>Mint API key</DialogTitle>
                  <DialogDescription>
                    The full key is shown only once after creation.
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="name">Name</Label>
                    <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
                  </div>
                  <div className="space-y-2">
                    <Label>Scope</Label>
                    <Select value={scope} onValueChange={(v) => setScope(v as Scope)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {scopeOptions.map((o) => (
                          <SelectItem key={o.value} value={o.value}>
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Account</Label>
                    <Select value={accountId} onValueChange={setAccountId}>
                      <SelectTrigger>
                        <SelectValue placeholder="Pick an account" />
                      </SelectTrigger>
                      <SelectContent>
                        {(accountsQuery.data ?? []).map((a) => (
                          <SelectItem key={a.id} value={a.id}>
                            {a.display_name} · {a.email}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Expires in</Label>
                    <Select value={expiresIn} onValueChange={setExpiresIn}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {expiryOptions.map((o) => (
                          <SelectItem key={o.value || "never"} value={o.value}>
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" disabled={create.isPending || !accountId}>
                    Mint
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        }
      />

      <Card>
        <CardContent className="p-0">
          {keysQuery.isLoading ? (
            <div className="space-y-3 p-6">
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ) : !keysQuery.data || keysQuery.data.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No keys yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Prefix</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-px" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {keysQuery.data.map((k) => (
                  <KeyRow key={k.id} k={k} onRevoke={() => revoke.mutate(k.id)} />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <MintedKeyDialog
        minted={minted}
        onClose={() => setMinted(null)}
        onCopied={() => toast.push({ title: "Key copied to clipboard", variant: "success" })}
      />
    </>
  );
}

function KeyRow({ k, onRevoke }: { k: APIKey; onRevoke: () => void }) {
  const status = k.revoked_at
    ? "revoked"
    : k.expires_at && new Date(k.expires_at).getTime() < Date.now()
      ? "expired"
      : "active";
  return (
    <TableRow>
      <TableCell className="font-medium">{k.name}</TableCell>
      <TableCell>
        <code className="text-xs text-muted-foreground">{k.key_prefix}</code>
      </TableCell>
      <TableCell>
        <Badge variant={k.scope === "admin" ? "destructive" : "secondary"}>{k.scope}</Badge>
      </TableCell>
      <TableCell>
        <Badge
          variant={
            status === "active" ? "success" : status === "expired" ? "warning" : "outline"
          }
        >
          {status}
        </Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{formatRelativeTime(k.created_at)}</TableCell>
      <TableCell>
        {status === "active" && (
          <Button
            size="sm"
            variant="ghost"
            className="text-destructive"
            onClick={onRevoke}
          >
            Revoke
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
}

function MintedKeyDialog({
  minted,
  onClose,
  onCopied,
}: {
  minted: APIKeyMintResponse | null;
  onClose: () => void;
  onCopied: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    if (!minted) {
      setCopied(false);
      setElapsed(0);
      return;
    }
    const start = Date.now();
    const interval = setInterval(() => {
      setElapsed(Math.floor((Date.now() - start) / 1000));
    }, 500);
    return () => clearInterval(interval);
  }, [minted]);

  const canClose = copied || elapsed >= 5;

  async function copy() {
    if (!minted) return;
    try {
      await navigator.clipboard.writeText(minted.key);
      setCopied(true);
      onCopied();
    } catch {
      // ignore clipboard errors — user can still select the text manually
    }
  }

  return (
    <Dialog open={Boolean(minted)} onOpenChange={(o) => !o && canClose && onClose()}>
      <DialogContent showClose={false}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert className="h-4 w-4 text-amber-400" /> Copy this key now
          </DialogTitle>
          <DialogDescription>
            The key is shown only once. After closing this dialog, only the prefix is visible.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-3">
          <code className="block break-all rounded-md border bg-muted/40 p-3 text-sm">
            {minted?.key}
          </code>
          {minted?.expires_at && (
            <p className="text-xs text-muted-foreground">
              Expires {new Date(minted.expires_at).toLocaleString()}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={copy}>
            <Copy className="mr-2 h-4 w-4" /> {copied ? "Copied" : "Copy"}
          </Button>
          <Button type="button" onClick={onClose} disabled={!canClose}>
            {canClose ? "Done" : `Wait ${Math.max(0, 5 - elapsed)}s…`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
