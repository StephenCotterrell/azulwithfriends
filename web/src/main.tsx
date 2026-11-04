import { createRoot } from "react-dom/client";
import { GuestPanel } from "./GuestPanel";
import "./styles.css";

function Welcome() {
  return (
    <main className="welcome">
      <div className="welcome-content">
        <div className="masthead" aria-hidden="true">
          <img src="/tile.svg" alt="" width="38" height="38" />
          <span />
        </div>
        <h1>Azul <em>with friends</em></h1>
        <GuestPanel />
      </div>
      <div className="tile-panel" aria-hidden="true">
        <img src="/courtyard.svg" alt="" width="620" height="660" />
      </div>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<Welcome />);
