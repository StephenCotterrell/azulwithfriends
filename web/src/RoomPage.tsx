import { useEffect, useEffectEvent, useRef, useState } from "react";
import { APIError, inviteURL, postJSON, requestJSON, roomName, roomPath, type Move, type Room } from "./api";
import { GuestPanel } from "./GuestPanel";
import { rememberGame } from "./recentGame";
import { GameTable, PlayGuide } from "./GameTable";

function canJoin(room: Room) {
  return room.yourSeat === 0 && room.seats.length === 1 && !room.game;
}

type RoomAction =
  | { name: "join" | "start"; body?: never }
  | { name: "close"; roomPath: string }
  | { name: "moves"; body: Move & { revision: number } }
  | { name: "listing"; body: { listed: boolean } }
  | { name: "bot"; body: { enabled: boolean } };

function roomReflectsAction(room: Room, action: RoomAction) {
  switch (action.name) {
    case "join": return !canJoin(room);
    case "start": return Boolean(room.game);
    case "moves": return (room.game?.revision ?? 0) > action.body.revision;
    case "listing": return room.listed === action.body.listed;
    case "bot": return room.seats.some(seat => seat.isBot) === action.body.enabled;
    case "close": return false; // A closed room returns 404.
  }
}

function roomPageVisible() {
  return !document.hidden && !("prerendering" in document && document.prerendering === true);
}

function CloseRoomDialog({ busy, error, onConfirm, onCancel }: {
  busy: boolean; error: string; onConfirm: () => void; onCancel: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { dialog.current?.showModal(); }, []);

  return (
    <dialog className="close-room-dialog" ref={dialog} aria-labelledby="close-room-heading"
      aria-describedby="close-room-description" onClose={onCancel} onCancel={event => { if (busy) event.preventDefault(); }}>
      <h2 id="close-room-heading">Leave and close this room?</h2>
      <p id="close-room-description">Closing the room permanently removes the game and its scores. You’ll return to the lobby.</p>
      {error && <p className="game-error" role="alert">{error}</p>}
      <div className="actions">
        <button type="button" className="secondary" disabled={busy} autoFocus onClick={() => dialog.current?.close()}>Cancel</button>
        <button type="button" className="confirm-close" disabled={busy} onClick={onConfirm}>{busy ? "Closing…" : "Leave and close room"}</button>
      </div>
    </dialog>
  );
}

function InviteLink({ url }: { url: string }) {
  const [notice, setNotice] = useState("");
  const input = useRef<HTMLInputElement>(null);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setNotice("Link copied.");
    } catch {
      input.current?.focus();
      input.current?.select();
      setNotice("Copy the selected link to share it.");
    }
  }

  return (
    <div className="invite">
      <label className="field-label" htmlFor="invite-link">Invite link</label>
      <input id="invite-link" type="text" value={url} ref={input} readOnly onFocus={event => event.target.select()} onBlur={() => setNotice("")} />
      <div className="actions">
        <button type="button" onClick={copy}>Copy invite link</button>
        <span className="hint" role="status">{notice}</span>
      </div>
    </div>
  );
}

function Seats({ room, pending, onBot, onRefresh }: {
  room: Room; pending: RoomAction | null; onBot: (enabled: boolean) => void; onRefresh: () => void;
}) {
  return (
    <section className="seats" aria-labelledby="players-heading">
      <div className="roster-art" aria-hidden="true" />
      <div className="seats-inner">
        <div className="roster-heading">
          <h2 id="players-heading">Players</h2>
          <button className="text-button" onClick={onRefresh} type="button" aria-label="Refresh room">Refresh</button>
        </div>
        <ol aria-label="Players">
          {[1, 2].map(number => {
            const seat = room.seats.find(player => player.number === number);
            const own = number === room.yourSeat;
            return (
              <li key={number} className={seat ? "occupied-seat" : "open-seat"}>
                <span className="seat-number" aria-label={`Seat ${number}`}>{number}</span>
                <span className="seat-name">{seat?.displayName ?? "Open seat"}</span>
                {own && <span className="you">You</span>}
                {room.yourSeat === 1 && number === 2 && (!seat || seat.isBot) && (
                  <button type="button" className="secondary seat-action" disabled={!!pending} onClick={() => onBot(!seat)}>
                    {pending?.name === "bot" ? (pending.body.enabled ? "Adding…" : "Removing…") : (seat ? "Remove bot" : "Add bot")}
                  </button>
                )}
              </li>
            );
          })}
        </ol>
      </div>
    </section>
  );
}

export function RoomPage({ id }: { id: string }) {
  const path = `/api/rooms/${encodeURIComponent(id)}`;
  const embedded = window.top !== window.self;
  const [room, setRoom] = useState<Room | null>(null);
  const [missing, setMissing] = useState(false);
  const [readError, setReadError] = useState("");
  const [actionError, setActionError] = useState("");
  const [retryJoin, setRetryJoin] = useState(false);
  const [pending, setPending] = useState<RoomAction | null>(null);
  const [confirmClose, setConfirmClose] = useState(false);
  const writing = useRef(false);
  const triedInvite = useRef(false);
  const currentRead = useRef<AbortController | null>(null);
  const uncertainWrite = useRef<RoomAction | null>(null);
  const [revision, setRevision] = useState(0);
  const [paused, setPaused] = useState(false);
  const refresh = () => setRevision(value => value + 1);
  const joinFromInvite = useEffectEvent(() => { void act({ name: "join" }); });

  const slug = room?.slug;
  const activeGameSlug = room?.yourSeat && room.game && !room.game.finished ? room.slug : null;
  useEffect(() => {
    if (!activeGameSlug) return;
    function rememberVisibleGame() {
      if (activeGameSlug && roomPageVisible()) rememberGame(activeGameSlug);
    }
    rememberVisibleGame();
    document.addEventListener("visibilitychange", rememberVisibleGame);
    return () => document.removeEventListener("visibilitychange", rememberVisibleGame);
  }, [activeGameSlug]);

  useEffect(() => {
    if (!slug) return;
    document.title = `${roomName(slug)} · Azul with friends`;
    const canonical = roomPath(slug);
    if (window.location.pathname !== canonical) {
      window.history.replaceState(window.history.state, "", canonical + window.location.search + window.location.hash);
    }
  }, [slug]);

  useEffect(() => {
    if (pending || missing) return;
    let controller: AbortController | undefined;
    let timer: number | undefined;
    let deadline: number;
    function stop() {
      window.clearTimeout(timer);
      controller?.abort();
    }
    async function read() {
      if (!roomPageVisible() || writing.current) return;
      const request = new AbortController();
      controller = currentRead.current = request;
      try {
        const uncertain = uncertainWrite.current;
        const lookup = uncertain?.name === "close" ? uncertain.roomPath : path;
        const snapshot = await requestJSON<Room>(lookup, { signal: request.signal });
        if (request.signal.aborted || !roomPageVisible()) return;
        setRoom(snapshot);
        setReadError("");
        if (uncertain) {
          const confirmed = roomReflectsAction(snapshot, uncertain);
          if (uncertain.name === "join") {
            setRetryJoin(!confirmed);
            setActionError(confirmed ? "" : "Couldn’t join the room. Try again.");
          } else if (uncertain.name === "close") {
            setActionError("This room is still open. Try closing it again.");
          } else {
            setActionError(confirmed ? "" : "The room is up to date. You can try again.");
          }
          uncertainWrite.current = null;
        }
        // Claim once per visit, only after a visible page has checked the guest's seat.
        if (!embedded && canJoin(snapshot) && !triedInvite.current) {
          triedInvite.current = true;
          joinFromInvite();
          return;
        }
        if (snapshot.game?.finished || (snapshot.yourSeat === 0 && snapshot.seats.length === 2)) return;
        if (!snapshot.game && snapshot.seats.length === 1 && !snapshot.listed && Date.now() >= deadline) {
          setPaused(true);
          return;
        }
        let delay = snapshot.seats.length === 2 && Date.now() < deadline ? 3500 : 7000;
        if (snapshot.listed && !snapshot.game && Date.now() >= deadline) delay = 20000;
        if (snapshot.game) delay = snapshot.game.turn === snapshot.yourSeat ? 12000 : 2000;
        timer = window.setTimeout(read, delay);
      } catch (cause) {
        if (request.signal.aborted) return;
        if (cause instanceof APIError && cause.status === 404) {
          if (uncertainWrite.current?.name === "close") {
            window.location.replace("/");
            return;
          }
          setRoom(null);
          setMissing(true);
        } else {
          setReadError("Connection interrupted. Reconnecting to the room…");
          timer = window.setTimeout(read, 5000);
        }
      }
    }
    function resume() {
      stop();
      if (!roomPageVisible()) return;
      deadline = Date.now() + 5 * 60 * 1000;
      setPaused(false);
      void read();
    }
    resume();
    document.addEventListener("visibilitychange", resume);
    document.addEventListener("prerenderingchange", resume);
    window.addEventListener("online", resume);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", resume);
      document.removeEventListener("prerenderingchange", resume);
      window.removeEventListener("online", resume);
    };
  }, [path, embedded, pending, missing, revision]);

  async function act(action: RoomAction) {
    if (writing.current) return;
    writing.current = true;
    currentRead.current?.abort();
    setPending(action);
    setActionError("");
    if (action.name === "join") setRetryJoin(false);
    if (action.name === "close") {
      uncertainWrite.current = null;
      setReadError("");
    }
    try {
      if (action.name === "close") {
        const result = await postJSON<{ closed: boolean }>(`${action.roomPath}/close`);
        if (result.closed !== true) throw new Error("Checking whether the room closed…");
        window.location.replace("/");
        return;
      }
      const updated = await postJSON<Room>(`${path}/${action.name}`, action.body ?? {});
      setRoom(updated);
      setReadError("");
    } catch (cause) {
      if (action.name === "join" && cause instanceof APIError && cause.status === 409) {
        // Another visitor took the last seat. The refresh below shows the full room.
        return;
      }
      if (!(cause instanceof APIError) || cause.status >= 500 || (action.name === "close" && cause.status === 404)) {
        uncertainWrite.current = action;
        setActionError(action.name === "close" ? "Checking whether the room closed…" : "Checking the latest room state…");
      } else {
        setActionError(cause instanceof Error ? cause.message : "Couldn’t update this room. Try again.");
        if (action.name === "join") setRetryJoin(true);
      }
    } finally {
      writing.current = false;
      setPending(null);
      refresh();
    }
  }

  const full = room?.seats.length === 2;
  const seated = Boolean(room?.yourSeat);
  const host = room?.yourSeat === 1;
  const bot = room?.seats.find(seat => seat.isBot);
  const closeRoomPath = host && bot && room?.game ? `/api/rooms/${encodeURIComponent(room.id)}` : null;
  const title = missing ? "Room not found" : room ? roomName(room.slug) : "Room for two";
  let status = "";
  if (host && !full) status = room?.listed ? "Waiting for a second player." : "Invite a friend or add a bot to the open seat.";
  else if (host && bot) status = `${bot.displayName} is ready to play.`;
  else if (seated && !host) status = "Waiting for the host to start.";
  else if (!seated && full) status = "Both seats are taken.";
  else if (!seated && !embedded && !retryJoin) status = "Joining room…";

  return (
    <main className={room?.game ? "room-page playing" : "room-page"}>
      <header className="room-header"><a href="/" aria-label="Azul with friends home"><img src="/tile.svg" alt="" width="34" height="34" />Azul <em>with friends</em></a>
        {room?.game && <span className="room-header-name">{roomName(room.slug)}</span>}
        {closeRoomPath && <button type="button" className="secondary leave-game" disabled={!!pending}
          onClick={() => { setActionError(""); setConfirmClose(true); }}>Leave game</button>}
      </header>
      {confirmClose && closeRoomPath && <CloseRoomDialog busy={pending?.name === "close"} error={readError || actionError}
        onConfirm={() => void act({ name: "close", roomPath: closeRoomPath })} onCancel={() => { setConfirmClose(false); setActionError(""); }} />}
      {actionError && !confirmClose && <p className="game-error" role="alert">{actionError}</p>}
      {readError && !confirmClose && !missing && <div className="read-error"><p role="alert">{readError}</p><button className="secondary" type="button" onClick={refresh}>Refresh room</button></div>}
      {room?.game ? <>
        <GameTable room={room} game={room.game} busy={!!pending} onMove={move => void act({ name: "moves", body: move })} />
        <div className="game-tools">
          <PlayGuide />
          <details className="room-settings"><summary>{bot ? "Room settings" : "Room and invite"}</summary>
            {!bot && <InviteLink url={inviteURL(room.slug)} />}
            <GuestPanel onSaved={refresh} />
          </details>
          <button type="button" className="text-button" onClick={refresh}>Refresh table</button>
        </div>
      </> : <div className="room-layout">
        <div className="room-content">
          <h1>{title}</h1>
          {missing && <p className="intro">Check the invitation link or <a href="/">create a new room</a>.</p>}
          {!missing && !room && !readError && <p role="status">Opening room…</p>}
          {room && <>
            {status && <p className="intro" role="status">{status}</p>}
            {seated ? <>
              {host && full && <button type="button" className="start-game" onClick={() => void act({ name: "start" })} disabled={!!pending}>{pending?.name === "start" ? "Starting…" : "Start game"}</button>}
              {!bot && <InviteLink url={inviteURL(room.slug)} />}
              {host && !full && <div className="listing-control">
                <label><input type="checkbox" checked={pending?.name === "listing" ? pending.body.listed : room.listed} disabled={!!pending}
                  onChange={event => void act({ name: "listing", body: { listed: event.target.checked } })} />List in public lobby</label>
                <p className="hint">Offer your open seat for one hour.</p>
              </div>}
              <GuestPanel onSaved={refresh} />
            </> : full ? <a href="/">Create your own room</a> :
              embedded ? <a href={roomPath(room.slug)} target="_blank" rel="noopener noreferrer">Open room</a> : retryJoin &&
              <button type="button" onClick={() => void act({ name: "join" })} disabled={!!pending}>Retry joining</button>}
          </>}
          {paused && !readError && <p className="hint">Automatic updates paused. Refresh to see the latest players.</p>}
        </div>
        {room && <Seats room={room} pending={pending} onBot={enabled => void act({ name: "bot", body: { enabled } })} onRefresh={refresh} />}
      </div>}
    </main>
  );
}
