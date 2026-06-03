import type {
  Account,
  AccountWithRole,
  APIKey,
  APIKeyMintResponse,
  AuditFilters,
  AuditResponse,
  BootstrapStatus,
  DashboardStats,
  Health,
  Invite,
  InviteAcceptInfo,
  InviteAcceptResponse,
  ListMemoriesResponse,
  ListTriplesResponse,
  Me,
  Memory,
  OrgPolicy,
  Guardrail,
  ChatStatus,
  ChatAnswer,
  OnboardingComplete,
  Project,
  ProjectCategory,
  RegisterResponse,
  Scope,
  Team,
  TeamDetail,
} from "@/lib/types";

interface LoginResponse {
  api_key: string;
  account_id: string;
  org_id: string;
  scope: Scope;
}

const TOKEN_KEY = "anchored_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown, message: string) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts?: { auth?: boolean },
): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const auth = opts?.auth ?? true;
  if (auth) {
    const token = getToken();
    if (token) headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    clearToken();
    if (location.pathname !== "/login") {
      location.replace("/login");
    }
    throw new ApiError(401, null, "unauthorized");
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const payload = text ? safeJSON(text) : null;
  if (!res.ok) {
    const msg = typeof payload === "object" && payload && "error" in payload
      ? String((payload as { error: unknown }).error)
      : `HTTP ${res.status}`;
    throw new ApiError(res.status, payload, msg);
  }
  return payload as T;
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export const api = {
  login: (email: string, password: string) =>
    request<LoginResponse>("POST", "/v1/auth/login", { email, password }, { auth: false }),

  register: (email: string, password: string, displayName: string, orgName: string, orgSlug?: string) =>
    request<RegisterResponse>("POST", "/v1/auth/register", {
      email,
      password,
      display_name: displayName,
      org_name: orgName,
      org_slug: orgSlug,
    }, { auth: false }),

  getMe: () => request<Me>("GET", "/v1/me"),
  getHealth: () => request<Health>("GET", "/v1/health", undefined, { auth: false }),
  getStats: () => request<DashboardStats>("GET", "/v1/stats"),

  getProjects: () => request<Project[]>("GET", "/v1/projects"),
  getProject: (id: string) => request<Project>("GET", `/v1/projects/${id}`),
  getProjectMemories: (id: string, limit: number, offset: number) =>
    request<ListMemoriesResponse>(
      "GET",
      `/v1/projects/${id}/memories?limit=${limit}&offset=${offset}`,
    ),
  getProjectGraph: (id: string, limit: number, offset: number) =>
    request<ListTriplesResponse>(
      "GET",
      `/v1/projects/${id}/graph?limit=${limit}&offset=${offset}`,
    ),
  // searchMemories runs server-side search. mode "semantic" uses vector KNN
  // (falls back to text on the server if embeddings are disabled); "text" runs
  // substring/keyword search. Returns a flat ranked array.
  searchMemories: (projectId: string, q: string, mode: "text" | "semantic", limit = 20) => {
    const params = new URLSearchParams({ project_id: projectId, q, mode, limit: String(limit) });
    return request<Memory[]>("GET", `/v1/memories/search?${params.toString()}`);
  },
  deleteProject: (id: string) => request<void>("DELETE", `/v1/projects/${id}`),

  getAccounts: () => request<AccountWithRole[]>("GET", "/v1/accounts"),
  createAccount: (email: string, displayName: string) =>
    request<Account & { created: boolean }>("POST", "/v1/accounts", {
      email,
      display_name: displayName,
    }),

  getTeams: () => request<Team[]>("GET", "/v1/teams"),
  getTeam: (id: string) => request<TeamDetail>("GET", `/v1/teams/${id}`),
  createTeam: (name: string, slug: string) =>
    request<Team>("POST", "/v1/teams", { name, slug }),
  addTeamMember: (teamId: string, accountId: string) =>
    request<void>("POST", `/v1/teams/${teamId}/members`, { account_id: accountId }),
  removeTeamMember: (teamId: string, accountId: string) =>
    request<void>("DELETE", `/v1/teams/${teamId}/members/${accountId}`),

  getAPIKeys: () => request<APIKey[]>("GET", "/v1/api-keys"),
  createAPIKey: (
    name: string,
    scope: Scope,
    accountId: string,
    expiresIn?: string,
  ) =>
    request<APIKeyMintResponse>("POST", "/v1/api-keys", {
      name,
      scope,
      account_id: accountId,
      expires_in: expiresIn ?? "",
    }),
  revokeAPIKey: (id: string) => request<void>("DELETE", `/v1/api-keys/${id}`),

  getAudit: (filters: AuditFilters) => {
    const q = new URLSearchParams();
    if (filters.project) q.set("project", filters.project);
    if (filters.actor) q.set("actor", filters.actor);
    if (filters.action) q.set("action", filters.action);
    if (filters.from) q.set("from", filters.from);
    if (filters.to) q.set("to", filters.to);
    if (filters.limit != null) q.set("limit", String(filters.limit));
    if (filters.offset != null) q.set("offset", String(filters.offset));
    return request<AuditResponse>("GET", `/v1/audit?${q.toString()}`);
  },

  // Bootstrap / onboarding
  getBootstrapStatus: () =>
    request<BootstrapStatus>("GET", "/v1/bootstrap-status", undefined, { auth: false }),
  completeOnboarding: (body: {
    org: { name: string; slug: string };
    admin: { email: string; password: string; display_name: string };
    projects: { name: string; category: ProjectCategory; repo_url?: string }[];
  }) => request<OnboardingComplete>("POST", "/v1/onboarding/complete", body, { auth: false }),

  // Invites
  getInvites: () => request<Invite[]>("GET", "/v1/invites"),
  createInvite: (email: string, display_name: string, role: string) =>
    request<{ id: string; invite_url: string; expires_at: string }>("POST", "/v1/invites", { email, display_name, role }),
  revokeInvite: (id: string) => request<void>("DELETE", `/v1/invites/${id}`),
  getInviteByToken: (token: string) =>
    request<InviteAcceptInfo>("GET", `/v1/invites/accept/${token}`, undefined, { auth: false }),
  acceptInvite: (token: string, password: string) =>
    request<InviteAcceptResponse>("POST", `/v1/invites/accept/${token}`, { password }, { auth: false }),

  // Account management
  updateAccount: (id: string, body: { display_name?: string; role?: string }) =>
    request<void>("PATCH", `/v1/accounts/${id}`, body),
  deleteAccount: (id: string) => request<void>("DELETE", `/v1/accounts/${id}`),
  getAccountProjects: (id: string) => request<Project[]>("GET", `/v1/accounts/${id}/projects`),
  setAccountProjects: (id: string, project_ids: string[]) =>
    request<void>("PUT", `/v1/accounts/${id}/projects`, { project_ids }),

  // Projects
  createProject: (body: { name: string; slug?: string; category: ProjectCategory; remote_key?: string; repo_url?: string }) =>
    request<Project>("POST", "/v1/projects", body),

  // Optional RAG chat
  getChatStatus: () => request<ChatStatus>("GET", "/v1/chat/status"),
  chat: (project_id: string, query: string) =>
    request<ChatAnswer>("POST", "/v1/chat", { project_id, query }),

  // Guardrail policy (org-level, admin only)
  getPolicy: () => request<OrgPolicy>("GET", "/v1/policies"),
  updatePolicy: (body: { blocked_categories: string[]; quality_threshold: number; near_dup_threshold: number }) =>
    request<OrgPolicy>("PUT", "/v1/policies", body),

  // Guardrail manager (org-level, admin only)
  listGuardrails: () => request<Guardrail[]>("GET", "/v1/guardrails"),
  createGuardrail: (body: { kind: string; value: string; label?: string; description?: string }) =>
    request<Guardrail>("POST", "/v1/guardrails", body),
  updateGuardrail: (id: string, body: { enabled?: boolean; value?: string; label?: string; description?: string }) =>
    request<Guardrail>("PATCH", `/v1/guardrails/${id}`, body),
  deleteGuardrail: (id: string) => request<void>("DELETE", `/v1/guardrails/${id}`),
};
