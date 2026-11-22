import { useEffect, useLayoutEffect, useState, useSyncExternalStore } from "react";

// The first-paint script in index.html reads the same preference.
const storageKey = "azul.theme.v1";
const systemScheme = window.matchMedia("(prefers-color-scheme: dark)");

function subscribeToSystemScheme(notify: () => void) {
  systemScheme.addEventListener("change", notify);
  return () => systemScheme.removeEventListener("change", notify);
}

function themePreference(value: string | null | undefined) {
  return value === "light" || value === "dark" ? value : "system";
}

export function ThemeToggle() {
  const [preference, setPreference] = useState(() => themePreference(document.documentElement.dataset.theme));
  const systemDark = useSyncExternalStore(subscribeToSystemScheme, () => systemScheme.matches);
  const dark = preference === "system" ? systemDark : preference === "dark";
  const nextTheme = dark ? "light" : "dark";
  const label = `Switch to ${nextTheme} mode`;

  useLayoutEffect(() => {
    const root = document.documentElement;
    if (preference === "system") delete root.dataset.theme;
    else root.dataset.theme = preference;

    const chrome = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (chrome) chrome.content = getComputedStyle(root).backgroundColor;
  }, [preference, dark]);

  useEffect(() => {
    function restore() {
      try {
        setPreference(themePreference(localStorage.getItem(storageKey)));
      } catch { /* Keep the current choice for this page. */ }
    }
    function onStorage(event: StorageEvent) {
      if (event.key === storageKey || event.key === null) restore();
    }
    function onShow(event: PageTransitionEvent) {
      if (event.persisted) restore();
    }
    window.addEventListener("storage", onStorage);
    window.addEventListener("pageshow", onShow);
    restore();
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("pageshow", onShow);
    };
  }, []);

  function toggle() {
    setPreference(nextTheme);
    try {
      localStorage.setItem(storageKey, nextTheme);
    } catch { /* The control still works for this page. */ }
  }

  return (
    <button className="theme-toggle" type="button" onClick={toggle} aria-label={label} title={label}>
      <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" focusable="false">
        {dark ? <>
          <circle cx="12" cy="12" r="4.5" />
          <path d="M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1.5 1.5 M17.5 17.5L19 19 M19 5l-1.5 1.5 M6.5 17.5L5 19"
            stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </> : <path d="M20.8 14.4A8.4 8.4 0 0 1 9.6 3.2a9.1 9.1 0 1 0 11.2 11.2Z" />}
      </svg>
    </button>
  );
}
