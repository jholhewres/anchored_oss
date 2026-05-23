import type {
  Account,
  AccountWithRole,
  APIKey,
  APIKeyMintResponse,
  AuditFilters,
  AuditResponse,
  DashboardStats,
  Health,
  ListMemoriesResponse,
  Me,
  Project,
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
};
