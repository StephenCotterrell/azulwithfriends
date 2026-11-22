import { useEffect, useLayoutEffect, useState } from "react";

// The first-paint script in index.html reads the same preference.
const storageKey = "azul.theme.v1";

function themePreference(value: string | null | undefined) {
  return value === "light" || value === "dark" ? value : "system";
}

export function ThemePicker() {
  const [preference, setPreference] = useState(() => themePreference(document.documentElement.dataset.theme));

  useLayoutEffect(() => {
    const root = document.documentElement;
    if (preference === "system") delete root.dataset.theme;
    else root.dataset.theme = preference;

    const chrome = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    function updateChrome() {
      if (chrome) chrome.content = getComputedStyle(root).backgroundColor;
    }
    updateChrome();
    if (preference !== "system") return;
    const scheme = window.matchMedia("(prefers-color-scheme: dark)");
    scheme.addEventListener("change", updateChrome);
    return () => scheme.removeEventListener("change", updateChrome);
  }, [preference]);

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

  function choose(value: string) {
    const choice = themePreference(value);
    setPreference(choice);
    try {
      if (choice === "system") localStorage.removeItem(storageKey);
      else localStorage.setItem(storageKey, choice);
    } catch { /* The control still works for this page. */ }
  }

  return <label className="theme-picker">
    Theme
    <select value={preference} onChange={event => choose(event.target.value)}>
      <option value="system">System</option>
      <option value="light">Light</option>
      <option value="dark">Dark</option>
    </select>
  </label>;
}
