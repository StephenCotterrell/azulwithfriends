import { useEffect, useEffectEvent, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { APIError, botPersonalities, inviteURL, postJSON, requestJSON, roomName, roomPath, seatName, type BotLevel, type Move, type Room } from "./api";
import { GuestPanel } from "./GuestPanel";
import { rememberGame } from "./recentGame";
import { GameTable, PlayGuide } from "./GameTable";
import { ThemeToggle } from "./ThemeToggle";

function canJoin(room: Room) {
  return room.yourSeat === 0 && room.seats.length === 1 && !room.game;
}

type BotSetup = { enabled: true; level: BotLevel } | { enabled: false };

type RoomAction =
  | { name: "join" | "start"; body?: never }
  | { name: "close"; roomPath: string }
  | { name: "moves"; body: Move & { revision: number } }
  | { name: "rematch"; body: { revision: number } }
  | { name: "listing"; body: { listed: boolean } }
  | { name: "bot"; body: BotSetup };

function roomReflectsAction(room: Room, action: RoomAction) {
  switch (action.name) {
    case "join": return !canJoin(room);
    case "start": return Boolean(room.game);
    case "moves":
    case "rematch": return (room.game?.revision ?? 0) > action.body.revision;
    case "listing": return room.listed === action.body.listed;
    case "bot": {
      const bot = room.seats.find(seat => seat.isBot);
      return action.body.enabled ? bot?.botLevel === action.body.level : !bot;
    }
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

function CopyRoomName({ name, url, className }: { name: string; url: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  const fallback = useRef<HTMLDialogElement>(null);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      setCopied(false);
      if (fallback.current && !fallback.current.open) fallback.current.showModal();
      return;
    }
    setCopied(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 2000);
  }

  return <>
    <button type="button" className={`copy-room-name ${className ?? ""}`} onClick={copy}
      title="Copy room link" aria-label={`Copy room link for ${name}`}>{name}</button>
    <span className="sr-only" role="status">{copied ? "Room link copied." : ""}</span>
    {createPortal(<>
      {copied && <div className="room-copy-toast" aria-hidden="true">Copied!</div>}
      <dialog className="close-room-dialog copy-room-dialog" ref={fallback} aria-label="Copy room link manually">
        <label className="field-label">Copy this room link
          <input readOnly value={url} autoFocus onFocus={event => event.currentTarget.select()} />
        </label>
        <button type="button" className="secondary" onClick={() => fallback.current?.close()}>Done</button>
      </dialog>
    </>, document.body)}
  </>;
}

function AddPlayer({ pending, onAdd }: { pending: RoomAction | null; onAdd: () => void }) {
  const [type, setType] = useState("bot");
  return <div className="seat-settings">
    <label className="field-label" htmlFor="player-type">Player type</label>
    <div className="add-player">
      <select id="player-type" value={type} disabled={!!pending} onChange={event => setType(event.target.value)}>
        <option value="bot">AI</option>
        <option value="human">Human</option>
      </select>
      {type === "bot"
        ? <button type="button" disabled={!!pending} onClick={onAdd}>{pending?.name === "bot" ? "Adding…" : "Add bot"}</button>
        : <a href="#invite-link">Get invite link</a>}
    </div>
  </div>;
}

function Seats({ room, pending, onBot, onStart, onRefresh }: {
  room: Room; pending: RoomAction | null; onBot: (setup: BotSetup) => void; onStart: () => void; onRefresh: () => void;
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
            const canEdit = room.yourSeat === 1 && number === 2;
            const level = pending?.name === "bot" && pending.body.enabled ? pending.body.level : seat?.botLevel ?? "competitive";
            return (
              <li key={number} className={seat ? "occupied-seat" : "open-seat"}>
                <div className="seat-summary">
                  <span className="seat-number" aria-label={`Seat ${number}`}>{number}</span>
                  <span className="seat-name">{seat ? (canEdit ? seat.displayName : seatName(seat)) : "Open seat"}</span>
                  {own && <span className="you">You</span>}
                  {canEdit && seat?.isBot && <button type="button" className="text-button seat-action" disabled={!!pending} onClick={() => onBot({ enabled: false })}>
                    {pending?.name === "bot" && !pending.body.enabled ? "Removing…" : "Remove bot"}
                  </button>}
                </div>
                {canEdit && !seat && <AddPlayer pending={pending} onAdd={() => onBot({ enabled: true, level: "noob" })} />}
                {canEdit && seat?.isBot && (
                  <div className="seat-settings">
                    <label className="field-label" htmlFor="bot-level">Personality</label>
                    <select id="bot-level" value={level} disabled={!!pending} aria-describedby="bot-description"
                      onChange={event => onBot({ enabled: true, level: event.target.value as BotLevel })}>
                      {Object.entries(botPersonalities).map(([value, profile]) => <option key={value} value={value}>{profile.name}</option>)}
                    </select>
                    <p className="hint" id="bot-description">{botPersonalities[level].description}</p>
                  </div>
                )}
              </li>
            );
          })}
        </ol>
        {room.yourSeat === 1 && room.seats.length === 2 && (
          <div className="roster-start">
            <button type="button" onClick={onStart} disabled={!!pending}>{pending?.name === "start" ? "Starting…" : "Start game"}</button>
          </div>
        )}
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
  const finishedBotGame = host && bot && room?.game?.finished ? room.game : null;
  const roomLink = room ? inviteURL(room.slug) : "";
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
        <ThemeToggle />
        {room?.game && <div className="room-header-game">
          <CopyRoomName key={room.slug} className="room-header-name" name={title} url={roomLink} />
          {closeRoomPath && <button type="button" className="secondary leave-game" disabled={!!pending}
            onClick={() => { setActionError(""); setConfirmClose(true); }}>Leave game</button>}
        </div>}
      </header>
      {confirmClose && closeRoomPath && <CloseRoomDialog busy={pending?.name === "close"} error={readError || actionError}
        onConfirm={() => void act({ name: "close", roomPath: closeRoomPath })} onCancel={() => { setConfirmClose(false); setActionError(""); }} />}
      {actionError && !confirmClose && <p className="game-error" role="alert">{actionError}</p>}
      {readError && !confirmClose && !missing && <div className="read-error"><p role="alert">{readError}</p><button className="secondary" type="button" onClick={refresh}>Refresh room</button></div>}
      {room?.game ? <>
        <GameTable room={room} game={room.game} busy={!!pending} onMove={move => void act({ name: "moves", body: move })}
          rematch={finishedBotGame ? {
            pending: pending?.name === "rematch",
            onStart: () => void act({ name: "rematch", body: { revision: finishedBotGame.revision } }),
          } : undefined} />
        <div className="game-tools">
          <PlayGuide />
          <details className="room-settings" name="game-tools"><summary>{bot ? "Room settings" : "Room and invite"}</summary>
            <div className="game-tool-panel">
              <p className="settings-room-name"><CopyRoomName key={room.slug} name={title} url={roomLink} /></p>
              {!bot && <InviteLink url={roomLink} />}
              <GuestPanel onSaved={refresh} />
            </div>
          </details>
          <button type="button" className="text-button" onClick={refresh}>Refresh table</button>
        </div>
      </> : <div className="room-layout">
        <div className="room-content">
          <h1>{room ? <CopyRoomName key={room.slug} name={title} url={roomLink} /> : title}</h1>
          {missing && <p className="intro">Check the invitation link or <a href="/">create a new room</a>.</p>}
          {!missing && !room && !readError && <p role="status">Opening room…</p>}
          {room && <>
            {status && <p className="intro" role="status">{status}</p>}
            {seated ? <>
              {!bot && <InviteLink url={roomLink} />}
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
        {room && <Seats room={room} pending={pending} onBot={body => void act({ name: "bot", body })}
          onStart={() => void act({ name: "start" })} onRefresh={refresh} />}
      </div>}
    </main>
  );
}
