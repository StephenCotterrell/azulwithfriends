import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { GuestPanel } from "./GuestPanel";
import { RoomPage } from "./RoomPage";
import { postJSON, type Room } from "./api";
import "./styles.css";

function Welcome() {
  const [creating, setCreating] = useState(false);
  const [savingName, setSavingName] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    function onShow(event: PageTransitionEvent) {
      if (event.persisted) setCreating(false);
    }
    window.addEventListener("pageshow", onShow);
    return () => window.removeEventListener("pageshow", onShow);
  }, []);

  async function createRoom() {
    if (creating || savingName) return;
    setCreating(true);
    setError("");
    try {
      const room = await postJSON<Room>("/api/rooms");
      window.location.assign(`/r/${encodeURIComponent(room.id)}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Couldn’t create the room. Try again.");
      setCreating(false);
    }
  }

  return (
    <main className="welcome">
      <div className="welcome-content">
        <div className="masthead" aria-hidden="true">
          <img src="/tile.svg" alt="" width="38" height="38" />
          <span />
        </div>
        <h1>Azul <em>with friends</em></h1>
        <div className="entry-panel">
          <p className="intro">Open a room for two and invite a friend with a link.</p>
          <button type="button" onClick={createRoom} disabled={creating || savingName}>{creating ? "Creating room…" : "Create a room"}</button>
          {error && <p className="error" role="alert">{error}</p>}
          <GuestPanel disabled={creating} onSaving={setSavingName} />
        </div>
      </div>
      <div className="tile-panel" aria-hidden="true">
        <img src="/courtyard.svg" alt="" width="620" height="660" />
      </div>
    </main>
  );
}

const roomPath = /^\/r\/([^/]+)\/?$/.exec(window.location.pathname);
if (roomPath) document.title = "Room · Azul with friends";
createRoot(document.getElementById("root")!).render(roomPath ? <RoomPage id={roomPath[1]} /> : <Welcome />);
