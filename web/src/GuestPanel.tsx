import { useEffect, useRef, useState, type FormEvent } from "react";
import { postJSON, requestJSON } from "./api";

type Session = { player: { displayName: string } | null };

type Props = {
  disabled?: boolean;
  onSaved?: () => void;
  onSaving?: (saving: boolean) => void;
};

export function GuestPanel({ disabled, onSaved, onSaving }: Props) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [saved, setSaved] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const field = useRef<HTMLInputElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    requestJSON<Session>("/api/me", { signal: controller.signal })
      .then(({ player }) => {
        if (!controller.signal.aborted) setName(player?.displayName ?? "");
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("Couldn’t load your current name. You can still save a new one.");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [open]);

  useEffect(() => {
    if (open && !loading) field.current?.focus();
  }, [open, loading]);

  function close() {
    setOpen(false);
    requestAnimationFrame(() => toggle.current?.focus());
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (loading || saving) return;
    const displayName = name.trim();
    if (!displayName || Array.from(displayName).length > 24 || /\p{Cc}/u.test(displayName)) {
      setError("Use a name with 1 to 24 visible characters.");
      field.current?.focus();
      return;
    }
    setError("");
    setSaving(true);
    onSaving?.(true);
    try {
      await postJSON<Session>("/api/session", { displayName });
      setSaved(true);
      onSaved?.();
      close();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Your name couldn’t be saved. Try again.");
    } finally {
      setSaving(false);
      onSaving?.(false);
    }
  }

  return (
    <div className="name-editor">
      <button type="button" className="text-button" ref={toggle} disabled={disabled || saving}
        aria-expanded={open} aria-controls={open ? "name-form" : undefined} onClick={() => {
          if (open) { close(); return; }
          setSaved(false);
          setLoading(true);
          setError("");
          setName("");
          setOpen(true);
        }}>Display name</button>
      {!open && saved && <span className="name-notice hint" role="status">Name saved.</span>}
      {open && <form id="name-form" onSubmit={save} noValidate>
        <label className="field-label" htmlFor="guest-name">Your display name</label>
        <input id="guest-name" name="displayName" type="text" ref={field} value={name}
          onChange={(event) => { setName(event.target.value); setError(""); }}
          autoComplete="nickname" aria-describedby={error ? "name-hint name-error" : "name-hint"} disabled={disabled || loading || saving} />
        <p id="name-hint" className="hint">Optional. This name follows you between rooms in this browser.</p>
        {error && <p id="name-error" className="error" role="alert">{error}</p>}
        <div className="actions">
          <button type="submit" disabled={disabled || loading || saving}>{saving ? "Saving…" : "Save name"}</button>
          <button type="button" className="secondary" disabled={saving} onClick={close}>Cancel</button>
        </div>
      </form>}
    </div>
  );
}
