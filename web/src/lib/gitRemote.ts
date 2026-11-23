/** Return an actionable validation message when a Git remote cannot identify a repository. */
export function repositoryError(value: string): string | null {
  const remote = value.trim();
  if (!remote) return "A Git repository remote is required.";
  if (/[\s\p{C}\p{Z}]/u.test(remote)) return "Repository remotes cannot contain spaces or invisible characters.";
  if (!remote.includes("://")) {
    const at = remote.indexOf("@");
    if (at <= 0) return "Enter a valid https://, ssh://, or git@host:owner/repo remote.";
    if (remote.slice(0, at) !== "git") return "SSH repository remotes may use only the git user.";
    const rest = remote.slice(at + 1);
    const colon = rest.indexOf(":");
    const host = rest.slice(0, colon);
    const path = rest.slice(colon + 1).replace(/^\/+|\/+$/g, "");
    return colon > 0 && host && path && !/[\/@:]/.test(host)
      ? null
      : "Enter a valid https://, ssh://, or git@host:owner/repo remote.";
  }

  if (!remote.startsWith("https://") && !remote.startsWith("ssh://")) {
    return "Use https:// or ssh:// for URL-style remotes.";
  }
  if (/%(?![0-9A-Fa-f]{2})/.test(remote)) return "Repository remotes cannot contain a malformed % escape.";
  // Read the user part as text, as the server does: an empty user or any
  // ":" in it (an empty password too) is refused.
  const afterScheme = remote.slice(remote.indexOf("://") + 3);
  const authority = afterScheme.split("/")[0];
  const at = authority.lastIndexOf("@");
  const userinfo = at >= 0 ? authority.slice(0, at) : null;
  if (userinfo !== null && (userinfo === "" || userinfo.includes(":"))) {
    return "Repository remotes cannot include a password or an empty user.";
  }
  if (remote.startsWith("ssh://") && userinfo !== null && userinfo !== "git") {
    return "SSH repository remotes may use only the git user.";
  }
  if (remote.startsWith("https://") && userinfo !== null && /:\d+$/.test(authority)) {
    return "Drop the username from an HTTPS remote that names a port.";
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
  // A bare HTTPS username (https://jdoe@host/...) is accepted; the server
  // drops it before storing.
  return null;
}
