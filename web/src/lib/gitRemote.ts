/** Return an actionable validation message when a Git remote cannot identify a repository. */
export function repositoryError(value: string): string | null {
  const remote = value.trim();
  if (!remote) return "A Git repository remote is required.";
  if (/[\s\x00-\x1f\x7f]/.test(remote)) return "Repository remotes cannot contain spaces or control characters.";
  if (!remote.includes("://")) {
    const at = remote.indexOf("@");
    if (at <= 0) return "Enter a valid https://, ssh://, or user@host:owner/repo remote.";
    const rest = remote.slice(at + 1);
    const colon = rest.indexOf(":");
    const host = rest.slice(0, colon);
    const path = rest.slice(colon + 1).replace(/^\/+|\/+$/g, "");
    return colon > 0 && host && path && !/[\/@:]/.test(host) && !remote.slice(0, at).includes(":")
      ? null
      : "Enter a valid https://, ssh://, or user@host:owner/repo remote.";
  }

  if (!remote.startsWith("https://") && !remote.startsWith("ssh://")) {
    return "Use https:// or ssh:// for URL-style remotes.";
  }
  let parsed: URL;
  try {
    parsed = new URL(remote);
  } catch {
    return "Enter a complete Git remote with a host and repository path.";
  }
  if (!parsed.hostname || !parsed.pathname.replace(/^\/+|\/+$/g, "")) {
    return "Enter a complete Git remote with a host and repository path.";
  }
  if (parsed.search || parsed.hash) return "Repository remotes cannot include query strings or fragments.";
  // A bare username (https://jdoe@host/...) is accepted; the server drops it
  // before storing. A password never is.
  if (parsed.protocol === "https:" && parsed.password) {
    return "HTTPS repository remotes cannot include a password.";
  }
  if (parsed.protocol === "https:" && remote.startsWith("https://@")) {
    return "Enter a complete Git remote with a host and repository path.";
  }
  if (parsed.protocol === "ssh:" && parsed.username && (parsed.username !== "git" || parsed.password)) {
    return "SSH repository remotes may use only the git user and cannot include a password.";
  }
  return null;
}
