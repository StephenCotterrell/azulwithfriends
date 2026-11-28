import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Ceramic, TilePatterns } from "./Ceramic";
import { EntrySheet, type EntryChoice } from "./EntrySheet";
import { GuestPanel } from "./GuestPanel";
import { Lobby } from "./Lobby";
import { RoomPage } from "./RoomPage";
import { TableIcon } from "./TableActions";
import { savedAttempt, storeAttempt, type SoloAttempt } from "./soloAttempt";
import { ThemeToggle } from "./ThemeToggle";
import { APIError, postJSON, requestJSON, roomName, roomPath, type BotLevel, type Color, type Room } from "./api";
import "./styles.css";
import "./entry.css";

const entryTiles: Color[] = ["blue", "yellow", "white", "red", "yellow", "black"];

function Welcome() {
  const [sheet, setSheet] = useState<EntryChoice | null>(null);
  const [entering, setEntering] = useState<string | null>(null);
  const [savingName, setSavingName] = useState(false);
  const [createError, setCreateError] = useState("");
  const [soloError, setSoloError] = useState("");
  const roomWrite = useRef(false);
  const [soloAttempt, setSoloAttempt] = useState(() => savedAttempt());
  const attemptStored = useRef(true);
  const [joinError, setJoinError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [resumeGame, setResumeGame] = useState<string | null>(null);
  const busy = entering !== null || savingName;

  useEffect(() => {
    function onShow(event: PageTransitionEvent) {
      if (event.persisted) {
        roomWrite.current = false;
        setEntering(null);
        setSheet(null);
        if (attemptStored.current) setSoloAttempt(previous => savedAttempt(previous));
      }
    }
    window.addEventListener("pageshow", onShow);
    return () => window.removeEventListener("pageshow", onShow);
  }, []);

  function beginEntry(target: string) {
    if (busy || roomWrite.current) return false;
    roomWrite.current = true;
    setEntering(target);
    return true;
  }

  function finishEntry() {
    roomWrite.current = false;
    setEntering(null);
  }

  function rememberAttempt(attempt: SoloAttempt | null) {
    setSoloAttempt(attempt);
    attemptStored.current = storeAttempt(attempt);
  }

  async function playComputer(level: BotLevel) {
    if (!beginEntry("solo")) return;
    setSoloError("");
    let submitted = false;
    try {
      // Establish the host cookie before the request that commits the board.
      await postJSON("/api/session/guest");
      const attempt = soloAttempt ?? { id: crypto.randomUUID(), level };
      rememberAttempt(attempt);
      submitted = true;
      const room = await postJSON<Room>("/api/solo-games", attempt);
      rememberAttempt(null);
      window.location.assign(roomPath(room.slug));
    } catch (cause) {
      // A lost reply or temporary failure can follow a committed game.
      if (submitted && cause instanceof APIError && (cause.status === 400 || cause.status === 409)) rememberAttempt(null);
      setSoloError(cause instanceof Error ? cause.message : "Couldn’t start the game. Try again.");
      finishEntry();
    }
  }

  async function createRoom(listed: boolean, fromLobby = false) {
    if (!beginEntry(fromLobby ? "public" : "create")) return;
    const reportError = fromLobby ? setJoinError : setCreateError;
    reportError("");
    try {
      const room = await postJSON<Room>("/api/rooms", { listed });
      window.location.assign(roomPath(room.slug));
    } catch (cause) {
      reportError(cause instanceof Error ? cause.message : "Couldn’t create the room. Try again.");
      finishEntry();
    }
  }

  async function joinRoom(slug: string) {
    if (!beginEntry(slug)) return;
    setJoinError("");
    const path = `/api/rooms/${encodeURIComponent(slug)}`;
    try {
      await postJSON<Room>(`${path}/join`, { fromLobby: true });
      window.location.assign(roomPath(slug));
    } catch (cause) {
      if (!(cause instanceof APIError) || cause.status >= 500) {
        // If the reply was lost after joining, recover the seat without posting twice.
        const latest = await requestJSON<Room>(path).catch(() => null);
        if (latest?.yourSeat) {
          window.location.assign(roomPath(slug));
          return;
        }
      }
      setJoinError(cause instanceof Error ? cause.message : "Couldn’t join the room. Try again.");
      setRefreshKey(value => value + 1);
      finishEntry();
    }
  }

  return (
    <main className="entry-home">
      <TilePatterns />
      <header>
        <div className="masthead">
          <Ceramic color="blue" />
          <span className="masthead-rule" />
          <ThemeToggle />
        </div>
        <h1>Azul <em>with friends</em></h1>
      </header>
      <section className="entry-choices" aria-label="Play options">
        {resumeGame && <a className="entry-resume" href={busy ? undefined : roomPath(resumeGame)} aria-disabled={busy || undefined}>
          <TableIcon name="history" />
          <span className="entry-choice-text"><strong>Resume game</strong><small>{roomName(resumeGame)}</small></span>
          <TableIcon name="forward" />
        </a>}
        <button type="button" className="entry-create" aria-haspopup="dialog" disabled={busy}
          onClick={event => { setCreateError(""); setSheet({ mode: "create", trigger: event.currentTarget }); }}>
          <span className="entry-plus" aria-hidden="true">+</span>
          <span className="entry-create-title">Create a game</span>
          <span className="entry-create-copy">Invite a friend or open a seat to the public.</span>
          <span className="entry-mosaic" aria-hidden="true">
            {entryTiles.map((color, i) => <Ceramic color={color} key={i} />)}
          </span>
        </button>
        <button type="button" className="entry-solo" aria-haspopup="dialog" disabled={busy}
          onClick={event => { if (!soloAttempt) setSoloError(""); setSheet({ mode: "solo", trigger: event.currentTarget }); }}>
          <span className="entry-bot-mark" aria-hidden="true">
            <svg width="23" height="23" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round" strokeLinecap="round">
              <path d="M12 3v3m-7 3a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2zM3 12v3m18-3v3M9 12h.01M15 12h.01M10 16h4" />
            </svg>
          </span>
          <span className="entry-choice-text"><strong>Play the computer</strong><small>Choose from seven opponents.</small></span>
          <TableIcon name="forward" />
        </button>
      </section>
      <Lobby entering={entering} disabled={busy} joinError={joinError} refreshKey={refreshKey}
        onJoin={joinRoom} onOpenPublic={() => createRoom(true, true)} onResumeGame={setResumeGame} />
      <GuestPanel disabled={entering !== null} onSaving={setSavingName} onSaved={() => setRefreshKey(value => value + 1)} />
      {sheet && <EntrySheet choice={sheet} busy={busy} error={sheet.mode === "solo" ? soloError : createError}
        pendingLevel={soloAttempt?.level} onClose={() => setSheet(null)} onCreate={createRoom} onSolo={playComputer} />}
    </main>
  );
}

const route = /^\/(?:r|room)\/([^/]+)\/?$/.exec(window.location.pathname);
if (route) document.title = "Room · Azul with friends";
createRoot(document.getElementById("root")!).render(route ? <RoomPage id={route[1]} /> : <Welcome />);
