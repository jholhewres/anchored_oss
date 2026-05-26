import React, { useState, useEffect } from "react";
import { Card, ScopeChip, Status, Btn, Input, Avatar, Table, Badge } from "@/ds/components";
import { I } from "@/ds/icons";
import { api } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import type { AccountWithRole, Project, Invite } from "@/lib/types";

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString();
}

function AccCell({ name, sub }: { name: string; sub?: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
      <Avatar name={name} size={28} />
      <div>
        <div style={{ fontSize: 13.5, fontWeight: 500 }}>{name}</div>
        {sub && <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>{sub}</div>}
      </div>
    </div>
  );
}

// ── Modal backdrop ──────────────────────────────────────────────────────────
function Modal({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed", inset: 0, zIndex: 50,
        background: "rgba(0,0,0,0.6)",
        display: "flex", alignItems: "center", justifyContent: "center",
      }}
    >
      <div onClick={e => e.stopPropagation()} style={{
        background: "var(--bg-2)", border: "1px solid var(--border)",
        borderRadius: "var(--radius-lg)", padding: 28, width: 440, maxHeight: "80vh",
        overflow: "auto", boxShadow: "0 24px 64px rgba(0,0,0,0.5)",
      }}>
        {children}
      </div>
    </div>
  );
}

function ModalTitle({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
      <div style={{ fontSize: 16, fontWeight: 500 }}>{children}</div>
      <button type="button" onClick={onClose} style={{
        border: 0, background: "transparent", color: "var(--text-dim)",
        cursor: "pointer", display: "inline-flex", padding: 4,
      }}><I.x size={16} /></button>
    </div>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)", letterSpacing: 0.4, textTransform: "uppercase" as const, marginBottom: 6 }}>
      {children}
    </div>
  );
}

// ── Invite modal ────────────────────────────────────────────────────────────
interface InviteModalProps {
  onClose: () => void;
  onCreated: (invite: Invite, url: string) => void;
}
function InviteModal({ onClose, onCreated }: InviteModalProps) {
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("sync");
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!email.trim() || !displayName.trim()) return;
    setSubmitting(true);
    try {
      const res = await api.createInvite(email.trim(), displayName.trim(), role);
      const fakeInvite: Invite = {
        id: res.id,
        org_id: "",
        email: email.trim(),
        display_name: displayName.trim(),
        role,
        expires_at: res.expires_at,
        created_at: new Date().toISOString(),
      };
      onCreated(fakeInvite, res.invite_url);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to send invite";
      toast.push({ title: "Invite failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal onClose={onClose}>
      <ModalTitle onClose={onClose}>Invite developer</ModalTitle>
      <form onSubmit={submit}>
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div>
            <FieldLabel>Name</FieldLabel>
            <Input full size="md" placeholder="Jane Doe" value={displayName}
              onChange={e => setDisplayName(e.target.value)} required autoFocus />
          </div>
          <div>
            <FieldLabel>Email</FieldLabel>
            <Input full size="md" type="email" placeholder="jane@acme.com" value={email}
              onChange={e => setEmail(e.target.value)} required />
          </div>
          <div>
            <FieldLabel>Role</FieldLabel>
            <select value={role} onChange={e => setRole(e.target.value)} style={{
              width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
              background: "var(--bg-input)", border: "1px solid var(--border)",
              borderRadius: "var(--radius)", color: "var(--text)", cursor: "pointer",
            }}>
              <option value="admin">admin</option>
              <option value="sync">sync</option>
              <option value="readonly">readonly</option>
            </select>
          </div>
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
            <Btn variant="primary" size="md" full>
              {submitting ? "Sending…" : "Send invite"}
            </Btn>
          </div>
        </div>
      </form>
    </Modal>
  );
}

// ── Edit account modal ──────────────────────────────────────────────────────
interface EditModalProps {
  account: AccountWithRole;
  onClose: () => void;
  onSaved: (updated: AccountWithRole) => void;
}
function EditModal({ account, onClose, onSaved }: EditModalProps) {
  const toast = useToast();
  const [displayName, setDisplayName] = useState(account.display_name);
  const [role, setRole] = useState(account.role);
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    try {
      await api.updateAccount(account.id, { display_name: displayName.trim(), role });
      onSaved({ ...account, display_name: displayName.trim(), role });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to update account";
      toast.push({ title: "Update failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal onClose={onClose}>
      <ModalTitle onClose={onClose}>Edit member</ModalTitle>
      <form onSubmit={submit}>
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div>
            <FieldLabel>Name</FieldLabel>
            <Input full size="md" value={displayName}
              onChange={e => setDisplayName(e.target.value)} required autoFocus />
          </div>
          <div>
            <FieldLabel>Role</FieldLabel>
            <select value={role} onChange={e => setRole(e.target.value)} style={{
              width: "100%", height: 34, padding: "0 10px", fontSize: 13.5,
              background: "var(--bg-input)", border: "1px solid var(--border)",
              borderRadius: "var(--radius)", color: "var(--text)", cursor: "pointer",
            }}>
              <option value="admin">admin</option>
              <option value="sync">sync</option>
              <option value="readonly">readonly</option>
            </select>
          </div>
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
            <Btn variant="primary" size="md" full>
              {submitting ? "Saving…" : "Save changes"}
            </Btn>
          </div>
        </div>
      </form>
    </Modal>
  );
}

// ── Manage projects modal ───────────────────────────────────────────────────
interface ManageProjectsModalProps {
  account: AccountWithRole;
  allProjects: Project[];
  onClose: () => void;
}
function ManageProjectsModal({ account, allProjects, onClose }: ManageProjectsModalProps) {
  const toast = useToast();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.getAccountProjects(account.id)
      .then(ps => setSelected(new Set(ps.map(p => p.id))))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [account.id]);

  function toggle(id: string) {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  async function save() {
    setSubmitting(true);
    try {
      await api.setAccountProjects(account.id, [...selected]);
      toast.push({ title: "Projects updated", variant: "success" });
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to save";
      toast.push({ title: "Save failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal onClose={onClose}>
      <ModalTitle onClose={onClose}>Manage projects — {account.display_name}</ModalTitle>
      {loading ? (
        <div style={{ color: "var(--text-dim)", fontSize: 13 }}>Loading…</div>
      ) : (
        <>
          <div style={{ display: "flex", flexDirection: "column", gap: 6, marginBottom: 20, maxHeight: 320, overflowY: "auto" }}>
            {allProjects.filter(p => !p.deleted_at).map(p => (
              <label key={p.id} style={{
                display: "flex", alignItems: "center", gap: 10,
                padding: "8px 10px", borderRadius: "var(--radius)",
                background: selected.has(p.id) ? "var(--accent-bg)" : "var(--bg-1)",
                border: `1px solid ${selected.has(p.id) ? "var(--accent-border)" : "var(--border)"}`,
                cursor: "pointer",
              }}>
                <input
                  type="checkbox"
                  checked={selected.has(p.id)}
                  onChange={() => toggle(p.id)}
                  style={{ accentColor: "var(--accent)" }}
                />
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 13.5, fontWeight: 500 }}>{p.name}</div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>{p.slug}</div>
                </div>
              </label>
            ))}
            {allProjects.filter(p => !p.deleted_at).length === 0 && (
              <div style={{ color: "var(--text-dim)", fontSize: 13, textAlign: "center", padding: "20px 0" }}>
                No projects yet.
              </div>
            )}
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
            <Btn variant="primary" size="md" full onClick={save}>
              {submitting ? "Saving…" : "Save"}
            </Btn>
          </div>
        </>
      )}
    </Modal>
  );
}

// ── Delete confirm modal ────────────────────────────────────────────────────
interface DeleteModalProps {
  account: AccountWithRole;
  onClose: () => void;
  onDeleted: (id: string) => void;
}
function DeleteModal({ account, onClose, onDeleted }: DeleteModalProps) {
  const toast = useToast();
  const [submitting, setSubmitting] = useState(false);

  async function confirm() {
    setSubmitting(true);
    try {
      await api.deleteAccount(account.id);
      onDeleted(account.id);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to delete";
      toast.push({ title: "Delete failed", description: msg, variant: "error" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal onClose={onClose}>
      <ModalTitle onClose={onClose}>Remove member</ModalTitle>
      <p style={{ fontSize: 14, color: "var(--text-muted)", marginBottom: 20 }}>
        Remove <strong>{account.display_name}</strong> ({account.email}) from the organisation? This cannot be undone.
      </p>
      <div style={{ display: "flex", gap: 8 }}>
        <Btn type="button" variant="outline" size="md" onClick={onClose}>Cancel</Btn>
        <Btn variant="danger" size="md" full onClick={confirm}>
          {submitting ? "Removing…" : "Remove member"}
        </Btn>
      </div>
    </Modal>
  );
}

// ── Main page ───────────────────────────────────────────────────────────────
export function DevelopersPage() {
  const toast = useToast();
  const [accounts, setAccounts] = useState<AccountWithRole[]>([]);
  const [invites, setInvites] = useState<Invite[]>([]);
  const [allProjects, setAllProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState("any");
  const [pendingOpen, setPendingOpen] = useState(true);

  const [showInviteModal, setShowInviteModal] = useState(false);
  const [editAccount, setEditAccount] = useState<AccountWithRole | null>(null);
  const [manageAccount, setManageAccount] = useState<AccountWithRole | null>(null);
  const [deleteAccount, setDeleteAccountModal] = useState<AccountWithRole | null>(null);

  // After invite creation: show the invite URL once
  const [freshInviteUrl, setFreshInviteUrl] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.getAccounts(),
      api.getInvites().catch(() => [] as Invite[]),
      api.getProjects().catch(() => [] as Project[]),
    ])
      .then(([accs, invs, projs]) => {
        setAccounts(accs);
        setInvites(invs.filter(inv => !inv.accepted_at));
        setAllProjects(projs);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const filtered = accounts.filter(a => {
    if (roleFilter !== "any" && a.role !== roleFilter) return false;
    if (search) {
      const q = search.toLowerCase();
      return a.email.toLowerCase().includes(q) || a.display_name.toLowerCase().includes(q);
    }
    return true;
  });

  function handleInviteCreated(invite: Invite, url: string) {
    setInvites(prev => [invite, ...prev]);
    setShowInviteModal(false);
    setFreshInviteUrl(url);
  }

  async function revokeInvite(id: string) {
    try {
      await api.revokeInvite(id);
      setInvites(prev => prev.filter(i => i.id !== id));
      toast.push({ title: "Invite revoked", variant: "success" });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to revoke";
      toast.push({ title: "Revoke failed", description: msg, variant: "error" });
    }
  }

  if (loading) return <div style={{ color: "var(--text-dim)", padding: 40 }}>Loading...</div>;

  return (
    <div>
      {/* Top bar */}
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16 }}>
        <Input icon={<I.search />} placeholder="Search by name or email…" size="sm" style={{ width: 320 }}
          value={search} onChange={e => setSearch(e.target.value)} />
        <select value={roleFilter} onChange={e => setRoleFilter(e.target.value)} style={{
          height: 28, padding: "0 8px", fontSize: 12,
          background: "var(--bg-input)", border: "1px solid var(--border)",
          borderRadius: "var(--radius)", color: "var(--text)", cursor: "pointer",
          fontFamily: "var(--font-mono)",
        }}>
          <option value="any">role: any</option>
          <option value="admin">admin</option>
          <option value="sync">sync</option>
          <option value="readonly">readonly</option>
        </select>
        <div style={{ flex: 1 }} />
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-dim)" }}>
          {filtered.length} members · {filtered.filter(a => a.role === "admin").length} admins
        </span>
        <Btn variant="primary" size="sm" icon={<I.plus />} onClick={() => setShowInviteModal(true)}>
          Invite developer
        </Btn>
      </div>

      {/* Fresh invite URL banner */}
      {freshInviteUrl && (
        <Card style={{ padding: 14, marginBottom: 16, border: "1px solid var(--ok-bg)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span style={{ color: "var(--ok)", display: "inline-flex" }}><I.check size={16} /></span>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 13.5, fontWeight: 500, marginBottom: 4 }}>Invite sent — copy the link</div>
              <div style={{
                fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--text-muted)",
                wordBreak: "break-all" as const,
              }}>{freshInviteUrl}</div>
            </div>
            <Btn variant="outline" size="sm" icon={<I.copy />}
              onClick={() => { navigator.clipboard.writeText(freshInviteUrl); }}>
              Copy
            </Btn>
            <button type="button" onClick={() => setFreshInviteUrl(null)} style={{
              border: 0, background: "transparent", color: "var(--text-dim)", cursor: "pointer",
              display: "inline-flex", padding: 4,
            }}><I.x size={14} /></button>
          </div>
        </Card>
      )}

      {/* Pending invites */}
      {invites.length > 0 && (
        <Card style={{ marginBottom: 16 }}>
          <button
            type="button"
            onClick={() => setPendingOpen(o => !o)}
            style={{
              width: "100%", display: "flex", alignItems: "center", justifyContent: "space-between",
              padding: "14px 18px", background: "transparent", border: 0, cursor: "pointer",
              color: "inherit", fontFamily: "inherit",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ fontSize: 14, fontWeight: 500 }}>Pending invites</span>
              <Badge tone="warn">{invites.length}</Badge>
            </div>
            {pendingOpen ? <I.chevU size={14} /> : <I.chevD size={14} />}
          </button>
          {pendingOpen && (
            <div style={{ borderTop: "1px solid var(--border)" }}>
              {invites.map((inv, i) => (
                <div key={inv.id} style={{
                  display: "grid", gridTemplateColumns: "1fr 1fr auto auto auto",
                  alignItems: "center", gap: 14, padding: "12px 18px",
                  borderBottom: i < invites.length - 1 ? "1px solid var(--border)" : "none",
                }}>
                  <div>
                    <div style={{ fontSize: 13.5, fontWeight: 500 }}>{inv.display_name}</div>
                    <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>{inv.email}</div>
                  </div>
                  <ScopeChip scope={inv.role} />
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-dim)" }}>
                    expires {formatDate(inv.expires_at)}
                  </div>
                  <Btn variant="outline" size="sm" onClick={() => revokeInvite(inv.id)}>Revoke</Btn>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {/* Members table */}
      {filtered.length === 0 ? (
        <Card style={{ padding: "40px 22px", textAlign: "center" }}>
          <div style={{ fontSize: 13, color: "var(--text-dim)" }}>No members found.</div>
        </Card>
      ) : (
        <Card>
          <Table
            cols={[
              { key: "name", label: "Member" },
              { key: "email", label: "Email", mono: true, muted: true },
              { key: "role", label: "Role" },
              { key: "created", label: "Joined", mono: true, muted: true },
              { key: "status", label: "Status" },
              { key: "actions", label: "", align: "right" as const },
            ]}
            rows={filtered.map(a => ({
              name: <AccCell name={a.display_name} />,
              email: a.email,
              role: <ScopeChip scope={a.role} />,
              created: timeAgo(a.created_at),
              status: <Status value="online" label="active" />,
              actions: (
                <div style={{ display: "flex", gap: 4, justifyContent: "flex-end" }}>
                  <Btn variant="ghost" size="sm" icon={<I.edit />} onClick={() => setEditAccount(a)} />
                  <Btn variant="ghost" size="sm" icon={<I.folder />} onClick={() => setManageAccount(a)} />
                  <Btn variant="ghost" size="sm" icon={<I.trash />} onClick={() => setDeleteAccountModal(a)} style={{ color: "var(--err)" }} />
                </div>
              ),
            }))}
          />
        </Card>
      )}

      {/* Modals */}
      {showInviteModal && (
        <InviteModal onClose={() => setShowInviteModal(false)} onCreated={handleInviteCreated} />
      )}
      {editAccount && (
        <EditModal
          account={editAccount}
          onClose={() => setEditAccount(null)}
          onSaved={updated => {
            setAccounts(prev => prev.map(a => a.id === updated.id ? updated : a));
            setEditAccount(null);
          }}
        />
      )}
      {manageAccount && (
        <ManageProjectsModal
          account={manageAccount}
          allProjects={allProjects}
          onClose={() => setManageAccount(null)}
        />
      )}
      {deleteAccount && (
        <DeleteModal
          account={deleteAccount}
          onClose={() => setDeleteAccountModal(null)}
          onDeleted={id => {
            setAccounts(prev => prev.filter(a => a.id !== id));
            setDeleteAccountModal(null);
          }}
        />
      )}
    </div>
  );
}
