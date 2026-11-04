import { useEffect, useRef, useState, type FormEvent } from "react";

type Player = { id: string; displayName: string };
type View =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "form"; previous: Player | null }
  | { kind: "saved"; player: Player };

async function requestSession(path: string, options?: RequestInit): Promise<Player | null> {
  const response = await fetch(path, {
    cache: "no-store",
    ...options,
  });
  const data = await response.json();
  if (!response.ok) {
    throw new Error(typeof data.error === "string" ? data.error : "Please try again.");
  }
  if (data.player === null) return null;
  if (typeof data.player?.id !== "string" || typeof data.player?.displayName !== "string") {
    throw new Error("The server returned an unexpected response.");
  }
  return data.player;
}

export function GuestPanel() {
  const [view, setView] = useState<View>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const field = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const controller = new AbortController();
    requestSession("/api/me", { signal: controller.signal })
      .then((player) => {
        if (!controller.signal.aborted) {
          setView(player ? { kind: "saved", player } : { kind: "form", previous: null });
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setView({ kind: "unavailable" });
      });
    return () => controller.abort();
  }, [attempt]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    const displayName = name.trim();
    if (!displayName || Array.from(displayName).length > 24 || /\p{Cc}/u.test(displayName)) {
      setError("Use a name with 1 to 24 visible characters.");
      field.current?.focus();
      return;
    }
    setError("");
    setSaving(true);
    try {
      const player = await requestSession("/api/session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ displayName }),
      });
      if (!player) throw new Error("Your guest name could not be saved.");
      setView({ kind: "saved", player });
    } catch (cause) {
      setError(cause instanceof Error && !(cause instanceof TypeError || cause instanceof SyntaxError)
        ? cause.message
        : "We couldn’t save your name. Check your connection and try again.");
    } finally {
      setSaving(false);
    }
  }

  if (view.kind === "loading") {
    return <section className="guest-panel" aria-label="Guest session"><p role="status">Checking your guest session…</p></section>;
  }
  if (view.kind === "unavailable") {
    return (
      <section className="guest-panel" aria-label="Guest session">
        <p role="alert">We couldn’t check your guest session.</p>
        <button type="button" onClick={() => {
          setView({ kind: "loading" });
          setAttempt(attempt + 1);
        }}>Try again</button>
      </section>
    );
  }
  if (view.kind === "saved") {
    return (
      <section className="guest-panel" aria-label="Your guest name">
        <p className="intro">This browser will remember your guest name.</p>
        <div className="guest-saved" role="status">
          <span className="check" aria-hidden="true">✓</span>
          <div><span className="field-label">Guest name</span><strong>{view.player.displayName}</strong></div>
        </div>
        <button className="secondary" type="button" onClick={() => {
          setName(view.player.displayName);
          setError("");
          setView({ kind: "form", previous: view.player });
        }}>Edit name</button>
      </section>
    );
  }
  return (
    <form className="guest-panel" onSubmit={save} noValidate>
      <p className="intro">Choose the name you’ll use at the table.</p>
      <label className="field-label" htmlFor="guest-name">Display name</label>
      <input id="guest-name" name="displayName" type="text" ref={field}
        value={name} onChange={(event) => { setName(event.target.value); setError(""); }}
        autoFocus={view.previous !== null} autoComplete="nickname" placeholder="Your name"
        aria-describedby={error ? "name-error" : "name-hint"} aria-invalid={error ? true : undefined}
        disabled={saving} />
      {error
        ? <p id="name-error" className="error" role="alert">{error}</p>
        : <p id="name-hint" className="hint">Saved for this browser.</p>}
      <div className="actions">
        <button type="submit" disabled={saving}>{saving ? "Saving…" : "Save guest name"}</button>
        {view.previous && <button className="secondary" type="button" disabled={saving} onClick={() => {
          if (view.previous) setView({ kind: "saved", player: view.previous });
        }}>Cancel</button>}
      </div>
    </form>
  );
}
