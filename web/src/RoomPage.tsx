import { useEffect, useRef, useState } from "react";
import { APIError, postJSON, requestJSON, type Room } from "./api";
import { GuestPanel } from "./GuestPanel";

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

function Seats({ room, onRefresh }: { room: Room; onRefresh: () => void }) {
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
  const [room, setRoom] = useState<Room | null>(null);
  const [missing, setMissing] = useState(false);
  const [readError, setReadError] = useState("");
  const [joinError, setJoinError] = useState("");
  const [joining, setJoining] = useState(false);
  const [revision, setRevision] = useState(0);
  const [paused, setPaused] = useState(false);
  const refresh = () => setRevision(value => value + 1);

  useEffect(() => {
    if (joining || missing) return;
    let controller: AbortController | undefined;
    let timer: number | undefined;
    let deadline: number;
    function stop() {
      window.clearTimeout(timer);
      controller?.abort();
    }
    async function read() {
      if (document.hidden) return;
      const request = new AbortController();
      controller = request;
      try {
        const snapshot = await requestJSON<Room>(path, { signal: request.signal });
        if (request.signal.aborted) return;
        setRoom(snapshot);
        setReadError("");
        if (snapshot.yourSeat === 0 && snapshot.seats.length === 2) return;
        if (Date.now() >= deadline) { setPaused(true); return; }
        timer = window.setTimeout(read, snapshot.seats.length < 2 ? 10000 : 30000);
      } catch (cause) {
        if (request.signal.aborted) return;
        if (cause instanceof APIError && cause.status === 404) {
          setRoom(null);
          setMissing(true);
        } else {
          setReadError("Couldn’t refresh this room. Check your connection and try again.");
        }
      }
    }
    function resume() {
      stop();
      if (document.hidden) return;
      deadline = Date.now() + 5 * 60 * 1000;
      setPaused(false);
      void read();
    }
    resume();
    document.addEventListener("visibilitychange", resume);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", resume);
    };
  }, [path, joining, missing, revision]);

  async function join() {
    if (joining) return;
    setJoining(true);
    setJoinError("");
    try {
      setRoom(await postJSON<Room>(`${path}/join`));
    } catch (cause) {
      setJoinError(cause instanceof Error ? cause.message : "Couldn’t join this room. Try again.");
    } finally {
      setJoining(false);
    }
  }

  const full = room?.seats.length === 2;
  const seated = Boolean(room?.yourSeat);
  const title = missing ? "Room not found" : "Room for two";
  const status = seated
    ? (full ? "Both seats are filled. Tile play is coming in a later release." : "Send the link to a friend to fill the other seat.")
    : (full ? "Both seats are taken." : "There’s a seat for you at this table.");

  return (
    <main className="room-page">
      <header className="room-header"><a href="/" aria-label="Azul with friends home"><img src="/tile.svg" alt="" width="34" height="34" />Azul <em>with friends</em></a></header>
      <div className="room-layout">
        <div className="room-content">
          <h1>{title}</h1>
          {missing && <p className="intro">Check the invitation link or <a href="/">create a new room</a>.</p>}
          {!missing && !room && !readError && <p role="status">Opening room…</p>}
          {room && <>
            <p className="intro" role="status">{status}</p>
            {seated ? <>
              <InviteLink url={new URL(`/r/${room.id}`, window.location.origin).href} />
              <GuestPanel onSaved={refresh} />
            </> : full ? <a href="/">Create your own room</a> :
              <button type="button" onClick={join} disabled={joining}>{joining ? "Joining…" : "Join room"}</button>}
            {joinError && <p className="error" role="alert">{joinError}</p>}
          </>}
          {readError && !missing && <div className="read-error"><p role="alert">{readError}</p><button className="secondary" type="button" onClick={refresh}>Try again</button></div>}
          {paused && !readError && <p className="hint">Automatic updates paused. Refresh to see the latest players.</p>}
        </div>
        {room && <Seats room={room} onRefresh={refresh} />}
      </div>
    </main>
  );
}
