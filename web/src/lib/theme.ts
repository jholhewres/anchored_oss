export type ThemeMode = "light" | "warm";

export const THEME_STORAGE_KEY = "anchored_theme";
export const THEME_MODES: ThemeMode[] = ["light", "warm"];

export function normalizeTheme(value: string | null | undefined): ThemeMode {
  return value === "warm" ? "warm" : "light";
}

export function readTheme(): ThemeMode {
  if (typeof window === "undefined") return "light";
  try {
    return normalizeTheme(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    return "light";
  }
}

export function applyTheme(theme: ThemeMode): ThemeMode {
  const next = normalizeTheme(theme);
  const root = document.documentElement;
  root.dataset.theme = next;
  root.style.colorScheme = next === "warm" ? "dark" : "light";
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, next);
  } catch {
    // localStorage can be unavailable in private/embedded contexts.
  }
  window.dispatchEvent(new CustomEvent("anchored-theme-change", { detail: next }));
  return next;
}

export function toggleTheme(theme: ThemeMode): ThemeMode {
  return theme === "light" ? "warm" : "light";
}
