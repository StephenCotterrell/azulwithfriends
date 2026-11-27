import { useEffect, useEffectEvent, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { requestJSON, type MatchHistory } from "./api";
import { MoveHistorySheet } from "./MoveHistorySheet";
import { HistoryBoard } from "./HistoryBoard";
import "./match-review.css";

// Room polling stays small. Fetch the committed history only while it has a reader.
export function useMatchHistory(roomId: string, matchId: string | undefined, revision: number,
  reader: "closed" | "results" | "review", refreshRoom: () => void) {
  const refresh = useEffectEvent(refreshRoom);
  const [saved, setSaved] = useState<MatchHistory | null>(null);
  const [failure, setFailure] = useState<{ matchId: string; message: string } | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (reader === "closed" || !matchId) return;
    const requestMatchId = matchId;
    const controller = new AbortController();
    let timer: number | undefined;
    async function read() {
      try {
        const result = await requestJSON<MatchHistory>(`/api/rooms/${encodeURIComponent(roomId)}/history`, { signal: controller.signal });
        if (controller.signal.aborted) return;
        if (result.matchId !== requestMatchId) {
          setFailure({ matchId: requestMatchId, message: "A new match has started. Refreshing the room…" });
          refresh();
          return;
        }
        setSaved(result);
        setFailure(null);
      } catch (e) {
        if (controller.signal.aborted) return;
        setFailure({ matchId: requestMatchId, message: e instanceof Error ? e.message : "History could not be loaded." });
        timer = window.setTimeout(read, 8000);
      }
    }
    void read();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [roomId, matchId, revision, reader, attempt]);
  return {
    history: saved?.matchId === matchId ? saved : null,
    error: failure && failure.matchId === matchId ? failure.message : "",
    retry: () => setAttempt(n => n + 1),
  };
}

export function HistoryStatus({ pending, error, retry }: { pending: boolean; error: string; retry: () => void }) {
  if (error) return <div className="history-status"><p role="status">{error}</p>
    <button type="button" className="secondary" onClick={retry}>Retry history</button></div>;
  return pending ? <p role="status" className="history-status">Loading match history…</p> : null;
}

type DialogProps = {
  history: MatchHistory | null; error: string; retry: () => void;
  names: [string, string]; yourSeat: number; returnLabel: string; interrupted: boolean; full: boolean;
  trigger: HTMLButtonElement; fallback: RefObject<HTMLHeadingElement | null>; onClose: () => void;
};

export function MatchReviewDialog({ history, error, retry, names, yourSeat, returnLabel, interrupted, full, trigger, fallback, onClose }: DialogProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [index, setIndex] = useState<number | null>(full ? 0 : null);
  const inspected = useRef<number | null>(null);
  // Freeze the visible list while reading. A new confirmed batch can be opened explicitly.
  const [frozen, setFrozen] = useState(history);
  const visible = frozen ?? history;
  const hasHistory = visible !== null;
  useEffect(() => {
    if (!frozen && history) setFrozen(history);
  }, [history, frozen]);
  useLayoutEffect(() => {
    const node = dialog.current!;
    node.showModal();
    return () => {
      node.close();
      (trigger.isConnected ? trigger : fallback.current)?.focus({ preventScroll: true });
    };
  }, [trigger, fallback]);
  useLayoutEffect(() => {
    const node = dialog.current;
    if (!node || !hasHistory) return;
    if (index !== null) node.querySelector<HTMLElement>("[data-history-back]")?.focus({ preventScroll: true });
    else if (inspected.current !== null) {
      node.querySelector<HTMLElement>(`[data-history-index="${inspected.current}"]`)?.focus({ preventScroll: true });
    } else node.querySelector<HTMLElement>("#move-history-title")?.focus({ preventScroll: true });
  }, [index, hasHistory]);

  function back() {
    if (!full && index !== null) setIndex(null);
    else onClose();
  }
  const boardOpen = index !== null && !!visible?.events.length;
  return <dialog ref={dialog} data-keep-draft className={`match-review ${boardOpen ? "review-board" : "review-sheet"}`}
    aria-label={boardOpen ? full ? "Match replay" : "Board snapshot" : "Move history"}
    onCancel={event => { event.preventDefault(); back(); }}
    onClick={event => {
      if (event.target !== dialog.current) return;
      const b = event.currentTarget.getBoundingClientRect();
      if (event.clientX < b.left || event.clientX > b.right || event.clientY < b.top || event.clientY > b.bottom) back();
    }}>
    {(!visible || error) && <HistoryStatus pending={!visible} error={error} retry={retry} />}
    {!visible && <button className="secondary review-dismiss" type="button" onClick={onClose}>Back to {full ? "final scores" : "game"}</button>}
    {visible && !full && <div className="review-list-pane" hidden={boardOpen}>
      {interrupted && <p role="status" className="review-notice">Live updates are paused. Showing recorded moves.</p>}
      {history && history.revision > visible.revision && <button type="button" className="review-notice secondary"
        onClick={() => { inspected.current = null; setFrozen(history); }}>Show updated history</button>}
      <MoveHistorySheet history={visible} names={names} yourSeat={yourSeat} returnLabel={returnLabel} onClose={onClose}
        onInspect={next => { inspected.current = next; setIndex(next); }} />
    </div>}
    {visible && boardOpen && <HistoryBoard history={visible} names={names} initialIndex={index!} full={full} onBack={back} />}
    {visible && full && !boardOpen && <div className="history-status"><p>No recorded positions are available for this match.</p>
      <button type="button" className="secondary" onClick={onClose}>Back to final scores</button></div>}
  </dialog>;
}
