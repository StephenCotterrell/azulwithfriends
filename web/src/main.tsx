import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { GuestPanel } from "./GuestPanel";
import { Lobby } from "./Lobby";
import { RoomPage } from "./RoomPage";
import { ThemeToggle } from "./ThemeToggle";
import { APIError, postJSON, requestJSON, roomName, roomPath, type Room } from "./api";
import "./styles.css";

function Welcome() {
  const [listed, setListed] = useState(false);
  const [entering, setEntering] = useState<string | null>(null);
  const [savingName, setSavingName] = useState(false);
  const [createError, setCreateError] = useState("");
  const [soloError, setSoloError] = useState("");
  const roomWrite = useRef(false);
  const soloAttempt = useRef<string | null>(null);
  const [joinError, setJoinError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [resumeGame, setResumeGame] = useState<string | null>(null);
  const busy = entering !== null || savingName;

  useEffect(() => {
    function onShow(event: PageTransitionEvent) {
      if (event.persisted) {
        roomWrite.current = false;
        setEntering(null);
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

  async function playComputer() {
    if (!beginEntry("solo")) return;
    setSoloError("");
    try {
      const id = soloAttempt.current ??= crypto.randomUUID();
      const room = await postJSON<Room>("/api/solo-games", { id });
      soloAttempt.current = null;
      window.location.assign(roomPath(room.slug));
    } catch (cause) {
      if (cause instanceof APIError && cause.status === 409) soloAttempt.current = null;
      setSoloError(cause instanceof Error ? cause.message : "Couldn’t start the game. Try again.");
      finishEntry();
    }
  }

  async function createRoom() {
    if (!beginEntry("create")) return;
    setCreateError("");
    try {
      const room = await postJSON<Room>("/api/rooms", { listed });
      window.location.assign(roomPath(room.slug));
    } catch (cause) {
      setCreateError(cause instanceof Error ? cause.message : "Couldn’t create the room. Try again.");
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
    <main className="welcome">
      <div className="welcome-content">
        <div className="masthead">
          <img src="/tile.svg" alt="" width="38" height="38" />
          <span className="masthead-rule" />
          <ThemeToggle />
        </div>
        <h1>Azul <em>with friends</em></h1>
        <div className="entry-panel">
          <div className="solo-entry">
            <button type="button" onClick={playComputer} disabled={busy}>
              {entering === "solo" ? "Starting game…" : "Play the computer"}
            </button>
            <span className="sr-only" role="status">{entering === "solo" && "Starting a game against the computer."}</span>
            {soloError && <p className="error" role="alert">{soloError}</p>}
          </div>
          {resumeGame && <a className="resume-game" href={busy ? undefined : roomPath(resumeGame)} aria-disabled={busy || undefined}>
            <strong>Resume game</strong>
            <span>{roomName(resumeGame)}</span>
          </a>}
          <fieldset className="room-visibility" disabled={busy}>
            <legend>Room visibility</legend>
            <div className="visibility-options">
              <label><input type="radio" name="visibility" checked={!listed} onChange={() => setListed(false)} />Link only</label>
              <label><input type="radio" name="visibility" checked={listed} onChange={() => setListed(true)} />Public lobby</label>
            </div>
          </fieldset>
          <p className="hint visibility-hint">{listed ? "Listed for one hour." : "Share the link with a friend, or add a bot."}</p>
          <button type="button" className="secondary" onClick={createRoom} disabled={busy}>{entering === "create" ? "Creating room…" : "Create a room"}</button>
          {createError && <p className="error" role="alert">{createError}</p>}
          <GuestPanel disabled={entering !== null} onSaving={setSavingName} onSaved={() => setRefreshKey(value => value + 1)} />
        </div>
      </div>
      <Lobby entering={entering} disabled={busy} joinError={joinError} refreshKey={refreshKey}
        onJoin={joinRoom} onResumeGame={setResumeGame} />
    </main>
  );
}

const route = /^\/(?:r|room)\/([^/]+)\/?$/.exec(window.location.pathname);
if (route) document.title = "Room · Azul with friends";
createRoot(document.getElementById("root")!).render(route ? <RoomPage id={route[1]} /> : <Welcome />);
