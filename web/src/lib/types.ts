export type Scope = "admin" | "sync" | "readonly";

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

export type ProjectCategory = "service" | "library" | "app" | "infra" | "experiment" | "other";
export const PROJECT_CATEGORIES: ProjectCategory[] = ["service", "library", "app", "infra", "experiment", "other"];

export const PROJECT_CATEGORY_LABELS: Record<ProjectCategory, string> = {
  service: "Services",
  library: "Libraries",
  app: "Apps",
  infra: "Infra",
  experiment: "Experiments",
  other: "Other",
};

export interface Project {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  remote_key: string;
  category: ProjectCategory;
  created_by: string;
  created_at: string;
  deleted_at?: string | null;
}

export interface Invite {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: string;
  expires_at: string;
  accepted_at?: string | null;
  created_by?: string;
  created_at: string;
}

export interface BootstrapStatus { bootstrapped: boolean; }

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_at: string;
}

export interface OnboardingComplete {
  api_key: string;
  org: Organization;
  admin: Account;
  projects: Project[];
}

export interface InviteAcceptInfo {
  valid: boolean;
  email: string;
  display_name: string;
}

export interface InviteAcceptResponse {
  api_key: string;
  account_id: string;
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
  metadata?: Record<string, unknown> | null;
}

export interface ChatStatus {
  enabled: boolean;
  model: string;
}

export interface ChatSource {
  id: string;
  category: string;
  snippet: string;
}

export interface ChatAnswer {
  answer: string;
  sources: ChatSource[];
}

export interface OrgPolicy {
  blocked_categories: string[];
  quality_threshold: number;
  near_dup_threshold: number;
  always_on: string[];
}

export type GuardrailKind =
  | "secret_detection"
  | "local_path_redaction"
  | "user_scope_block"
  | "category"
  | "regex"
  | "keyword";

export interface Guardrail {
  id: string;
  org_id: string;
  kind: GuardrailKind;
  value: string;
  label: string;
  description: string;
  enabled: boolean;
  builtin: boolean;
  created_at: string;
  updated_at: string;
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
