import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";

const preferenceKey = "azul.draftHints.v1";

function readHints() {
  try {
    return localStorage.getItem(preferenceKey) !== "off";
  } catch {
    return true;
  }
}

export function useDraftHints() {
  const [enabled, setEnabled] = useState(readHints);
  useEffect(() => {
    function sync(event: StorageEvent) {
      if (event.key === preferenceKey || event.key === null) setEnabled(readHints());
    }
    window.addEventListener("storage", sync);
    return () => window.removeEventListener("storage", sync);
  }, []);

  function setHints(value: boolean) {
    setEnabled(value);
    try {
      localStorage.setItem(preferenceKey, value ? "on" : "off");
    } catch {
      // Browsers with storage disabled still keep the preference for this game.
    }
  }
  return [enabled, setHints] as const;
}

const paths = {
  history: "M3 11a9 9 0 1 1 2.7 7.5M3 4v7h7M12 7v5l3 2",
  pick: "M3 8V3h5m8 0h5v5m0 8v5h-5M8 21H3v-5",
  place: "M4 5v8h15m-6-6 6 6-6 6",
  help: "M4 4h4c2 0 4 1 4 3 0-2 2-3 4-3h4v14h-4c-2 0-4 1-4 3 0-2-2-3-4-3H4zM12 7v14",
  settings: "M4 7h9m4 0h3M4 17h3m4 0h9M13 4v6M7 14v6",
  leave: "M10 4H4v16h6m3-4 4-4-4-4m-5 4h13",
  forward: "m9 5 7 7-7 7",
  back: "m15 5-7 7 7 7",
  interruption: "m12 3 9 17H3zM12 9v4m0 3v.2",
  more: "M4 12h.01m7.99 0h.01m7.99 0h.01",
} as const;

export function TableIcon({ name }: { name: keyof typeof paths }) {
  return <svg aria-hidden="true" focusable="false" className="table-icon" viewBox="0 0 24 24"
    fill="none" stroke="currentColor" strokeWidth={name === "more" ? 4 : 1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d={paths[name]} />
  </svg>;
}

const paneTitles = { menu: "Table actions", help: "How to play", settings: "Room settings" };
type Pane = keyof typeof paneTitles;

export function TableActions({ trigger, hints, onHints, onDone, onLeave, busy, help, settings }: {
  trigger: RefObject<HTMLButtonElement | null>;
  hints: boolean; onHints: (enabled: boolean) => void; onDone: () => void;
  onLeave?: () => void; busy: boolean; help: ReactNode; settings: ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const done = useRef<HTMLButtonElement>(null);
  const returnTo = useRef<Pane | null>(null);
  const [pane, setPane] = useState<Pane>("menu");

  useLayoutEffect(() => {
    const node = dialog.current;
    const button = trigger.current;
    if (!node || !button) return;
    const position = () => {
      const edge = button.getBoundingClientRect();
      const top = Math.max(6, Math.min(edge.bottom + 5, window.innerHeight - 220));
      node.style.setProperty("--menu-top", top + "px");
      node.style.setProperty("--menu-right", Math.max(8, window.innerWidth - edge.right) + "px");
    };
    position();
    node.showModal();
    done.current?.focus({ preventScroll: true });
    window.addEventListener("resize", position);
    return () => window.removeEventListener("resize", position);
  }, [trigger]);

  useLayoutEffect(() => {
    if (pane !== "menu") {
      heading.current?.focus({ preventScroll: true });
    } else if (returnTo.current) {
      // Menu buttons are remounted when returning from a pane.
      dialog.current?.querySelector<HTMLButtonElement>(`[data-open-pane="${returnTo.current}"]`)?.focus();
      returnTo.current = null;
    }
  }, [pane]);

  return <dialog ref={dialog} id="table-actions" data-keep-draft className="table-actions" aria-labelledby="table-actions-heading"
    onClose={onDone} onClick={event => {
      if (event.target !== event.currentTarget) return;
      const { left, right, top, bottom } = event.currentTarget.getBoundingClientRect();
      if (event.clientX < left || event.clientX > right || event.clientY < top || event.clientY > bottom) event.currentTarget.close();
    }}>
    <header className="actions-heading">
      {pane !== "menu" && <button type="button" className="pane-back" aria-label="Back to table actions" onClick={() => setPane("menu")}><TableIcon name="back" /></button>}
      <h2 id="table-actions-heading" ref={heading} tabIndex={-1}>{paneTitles[pane]}</h2>
      <button ref={done} className="text-button pane-done" type="button" onClick={() => dialog.current?.close()}>Done</button>
    </header>
    {pane === "menu" ? <div className="actions-list">
      <button type="button" role="switch" aria-checked={hints} aria-label="Draft hints" aria-describedby="hint-switch-detail"
        className="draft-hints-switch" onClick={() => onHints(!hints)}>
        <span className="switch-label"><strong>Draft hints</strong><small id="hint-switch-detail">Show pick and placement prompts.</small></span>
        <span className="switch-state" aria-hidden="true">{hints ? "On" : "Off"}</span>
        <span className="switch-track" aria-hidden="true"><span /></span>
      </button>
      {(["help", "settings"] as const).map(target =>
        <button key={target} className="pane-action" type="button" data-open-pane={target} onClick={() => {
          returnTo.current = target;
          setPane(target);
        }}>
          <TableIcon name={target} /><span>{paneTitles[target]}</span><TableIcon name="forward" />
        </button>
      )}
      {onLeave && <button className="pane-action table-leave" type="button" disabled={busy} onClick={onLeave}>
        <TableIcon name="leave" /><span>Leave game…</span>
      </button>}
    </div> : <div className="pane-content">{pane === "help" ? help : settings}</div>}
  </dialog>;
}
