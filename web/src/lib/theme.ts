// Light/dark toggle — sets [data-theme] on <html>, which index.css and
// accesslens-dashboard.css both key their dark-mode token overrides off of.
// Persisted per-browser via localStorage; falls back to the OS setting
// (prefers-color-scheme) until the user picks explicitly.
const STORAGE_KEY = "accesslens_theme";

export type Theme = "light" | "dark";

function systemPrefersDark(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function storedTheme(): Theme | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    return v === "light" || v === "dark" ? v : null;
  } catch {
    return null; // private mode etc. — just won't persist
  }
}

export function getInitialTheme(): Theme {
  return storedTheme() ?? (systemPrefersDark() ? "dark" : "light");
}

export function applyTheme(theme: Theme) {
  document.documentElement.setAttribute("data-theme", theme);
}

export function storeTheme(theme: Theme) {
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // private mode etc. — theme still applies for this load, just won't persist
  }
}
