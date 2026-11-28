import { useLayoutEffect, useRef, useState } from "react";
import { Ceramic } from "./Ceramic";
import { botPersonalities, type BotLevel, type Color } from "./api";

export type EntryChoice = { mode: "create" | "solo"; trigger: HTMLButtonElement };

type Props = {
  choice: EntryChoice;
  busy: boolean;
  error: string;
  pendingLevel: BotLevel | undefined;
  onClose: () => void;
  onCreate: (listed: boolean) => void;
  onSolo: (level: BotLevel) => void;
};

const levels = Object.keys(botPersonalities) as BotLevel[];
const swatches: Color[] = ["blue", "white", "yellow", "red", "black", "yellow", "blue"];

export function EntrySheet({ choice, busy, error, pendingLevel, onClose, onCreate, onSolo }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const [listed, setListed] = useState(false);
  const [level, setLevel] = useState<BotLevel>(pendingLevel ?? "beginner");
  const picking = choice.mode === "solo";

  useLayoutEffect(() => {
    const node = dialog.current!;
    node.showModal();
    heading.current?.focus({ preventScroll: true });
    return () => {
      node.close();
      choice.trigger.focus({ preventScroll: true });
    };
  }, [choice]);

  return <dialog className="entry-sheet" ref={dialog} aria-labelledby="entry-sheet-title" aria-describedby="entry-sheet-intro"
    onCancel={event => { if (busy) event.preventDefault(); }} onClose={onClose}
    onClick={event => {
      if (busy || event.target !== event.currentTarget) return;
      const bounds = event.currentTarget.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) {
        event.currentTarget.close();
      }
    }}>
    <form onSubmit={event => {
      event.preventDefault();
      if (picking) onSolo(level);
      else onCreate(listed);
    }}>
      <div className="entry-sheet-head">
        <h2 id="entry-sheet-title" ref={heading} tabIndex={-1}>{picking ? "Choose an opponent" : "Create a game"}</h2>
        <button className="entry-close" type="button" aria-label="Close" disabled={busy} onClick={() => dialog.current?.close()}>×</button>
      </div>
      <p id="entry-sheet-intro">{picking ? "Pick the playing style you want to face." : "Who can take the second seat?"}</p>
      <div className="entry-options-scroll">
        <fieldset className={picking ? "entry-options opponents" : "entry-options"} disabled={busy || (picking && Boolean(pendingLevel))}>
          <legend className="sr-only">{picking ? "Computer opponent" : "Room visibility"}</legend>
          {picking ? levels.map((key, i) => <label key={key} className="entry-option">
            <Ceramic color={swatches[i % swatches.length]} />
            <span className="entry-option-text"><strong>{botPersonalities[key].name}</strong><small>{botPersonalities[key].description}</small></span>
            <input type="radio" name="opponent" value={key} checked={level === key} onChange={() => setLevel(key)} />
          </label>) : <>
            <label className="entry-option">
              <input type="radio" name="visibility" value="private" checked={!listed} onChange={() => setListed(false)} />
              <span className="entry-option-text"><strong>Link only</strong><small>A friend can join with your invite link.</small></span>
            </label>
            <label className="entry-option">
              <input type="radio" name="visibility" value="public" checked={listed} onChange={() => setListed(true)} />
              <span className="entry-option-text"><strong>Public lobby</strong><small>List the open seat for up to one hour.</small></span>
            </label>
          </>}
        </fieldset>
      </div>
      {error && <p className="error" role="alert">{error}</p>}
      <button type="submit" className="entry-submit" disabled={busy}>
        {picking ? (busy ? "Starting game…" : `${pendingLevel ? "Retry" : "Start"} against ${botPersonalities[level].name}`)
          : (busy ? "Creating game…" : listed ? "Create public game" : "Create private game")}
      </button>
    </form>
  </dialog>;
}
