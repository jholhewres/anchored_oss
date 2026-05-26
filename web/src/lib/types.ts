export type Scope = "admin" | "sync" | "readonly";
export type Mode = "cloud" | "selfhosted";

export interface ModeResponse {
  mode: Mode;
}

export interface Me {
  account_id: string;
  org_id: string;
  scope: Scope;
  email: string;
  display_name: string;
}

export interface DashboardStats {
  accounts: number;
  organizations: number;
  teams: number;
  projects: number;
  memories_live: number;
  keys_active: number;
  audit_entries_24h: number;
  recent_pushes: PushActivity[];
}

export interface PushActivity {
  project_id: string;
  project_name: string;
  count: number;
  last_push: string;
}

export interface Project {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  remote_key: string;
  created_by: string;
  created_at: string;
  deleted_at?: string | null;
}

export interface Account {
  id: string;
  email: string;
  display_name: string;
  created_at: string;
}

export interface AccountWithRole extends Account {
  role: string;
}

export interface Team {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  created_at: string;
}

export interface TeamMember {
  account_id: string;
  email: string;
  display_name: string;
  added_at: string;
}

export interface ProjectGrant {
  project_id: string;
  project_name: string;
  project_slug: string;
  role: string;
}

export interface TeamDetail extends Team {
  members: TeamMember[];
  project_grants: ProjectGrant[];
}

export interface APIKey {
  id: string;
  org_id: string;
  account_id: string;
  name: string;
  key_prefix: string;
  scope: Scope;
  expires_at?: string | null;
  created_at: string;
  revoked_at?: string | null;
}

export interface APIKeyMintResponse {
  id: string;
  name: string;
  key: string;
  scope: Scope;
  created_at: string;
  expires_at?: string;
}

export interface Memory {
  id: string;
  project_id: string;
  category: string;
  content: string;
  content_hash: string;
  keywords?: string[];
  source?: string;
  author_id?: string;
  author_name?: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string | null;
}

export interface ListMemoriesResponse {
  memories: Memory[];
  total: number;
  limit: number;
  offset: number;
}

export interface Triple {
  id: string;
  subject: string;
  predicate: string;
  object: string;
  confidence: number;
  project_id: string;
  created_at: string;
}

export interface ListTriplesResponse {
  triples: Triple[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditEntry {
  id: string;
  org_id: string;
  project_id?: string;
  actor_id?: string;
  action: string;
  target_type?: string;
  target_id?: string;
  metadata?: unknown;
  created_at: string;
}

export interface AuditResponse {
  entries: AuditEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditFilters {
  project?: string;
  actor?: string;
  action?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface RegisterResponse {
  api_key: string;
  account_id: string;
  org_id: string;
  scope: Scope;
}

export interface Health {
  service: string;
  version: string;
  status: string;
  db_status: string;
  timestamp: string;
}
