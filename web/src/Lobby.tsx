import { useEffect, useState } from "react";
import { requestJSON, roomName, roomPath, type LobbyRoom } from "./api";
import { recentGame } from "./recentGame";

type Props = {
  entering: string | null;
  disabled: boolean;
  joinError: string;
  refreshKey: number;
  onJoin: (slug: string) => void;
  onOpenPublic: () => void;
  onResumeGame: (slug: string | null) => void;
};

export function Lobby({ entering, disabled, joinError, refreshKey, onJoin, onOpenPublic, onResumeGame }: Props) {
  const [rooms, setRooms] = useState<LobbyRoom[] | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [paused, setPaused] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let timer: number | undefined;
    let request: AbortController | undefined;
    let deadline: number;

    async function read() {
      const controller = new AbortController();
      request = controller;
      try {
        const preferred = recentGame();
        const data = await requestJSON<{ rooms: LobbyRoom[]; resumeGame?: string }>("/api/lobby", {
          signal: controller.signal,
          headers: preferred ? { "X-Recent-Game": preferred } : undefined,
        });
        if (controller.signal.aborted) return;
        setRooms(data.rooms);
        onResumeGame(data.resumeGame ?? null);
        setLoadError(false);
      } catch {
        if (controller.signal.aborted) return;
        setLoadError(true);
        onResumeGame(null);
      }
      if (Date.now() < deadline) timer = window.setTimeout(read, 15000);
      else setPaused(true);
    }
    function stop() {
      window.clearTimeout(timer);
      request?.abort();
    }
    function resume() {
      stop();
      if (document.hidden) {
        onResumeGame(null);
        return;
      }
      setPaused(false);
      deadline = Date.now() + 5 * 60 * 1000;
      void read();
    }
    function onShow(event: PageTransitionEvent) {
      if (event.persisted) {
        onResumeGame(null);
        resume();
      }
    }
    resume();
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("pageshow", onShow);
    window.addEventListener("online", resume);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("pageshow", onShow);
      window.removeEventListener("online", resume);
    };
  }, [attempt, refreshKey, onResumeGame]);

  return (
    <section className="lobby" aria-labelledby="lobby-title">
      <div className="lobby-heading">
        <h2 id="lobby-title">Public lobby</h2>
        <button className="text-button" type="button" aria-label="Refresh lobby" onClick={() => setAttempt(value => value + 1)}>Refresh</button>
      </div>
      {joinError && <p className="error" role="alert">{joinError}</p>}
      {loadError && <p className="error" role="status">Couldn’t update the lobby. Refresh to try again.</p>}
      {rooms === null && !loadError && <p className="lobby-status" role="status">Looking for open seats…</p>}
      {rooms?.length === 0 && !loadError && <p className="lobby-status">No open games right now.</p>}
      {rooms && rooms.length > 0 && <ul className="lobby-list" aria-label="Open rooms">
        {rooms.map(({ slug, hostName, isHost }) => {
          const name = roomName(slug);
          return <li key={slug}>
            <div>
              <span className="lobby-room-name">{name}</span>
              <span className="lobby-host">{isHost ? "Your room" : `Hosted by ${hostName}`} · 1 seat open</span>
            </div>
            {isHost ? <a className="lobby-return" href={disabled ? undefined : roomPath(slug)} aria-disabled={disabled || undefined} aria-label={`Return to room ${name}`}>Return to room</a> :
              <button className="secondary" type="button" aria-label={`Join ${name}`} disabled={disabled} onClick={() => onJoin(slug)}>
                {entering === slug ? "Joining…" : "Join"}
              </button>}
          </li>;
        })}
      </ul>}
      <button className="secondary lobby-open" type="button" onClick={onOpenPublic} disabled={disabled}>
        {entering === "public" ? "Opening game…" : "Open a public game"}
      </button>
      {paused && !loadError && <p className="hint" role="status">Live updates paused. Refresh for open seats.</p>}
    </section>
  );
}
