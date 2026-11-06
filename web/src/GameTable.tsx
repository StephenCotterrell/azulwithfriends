import { useLayoutEffect, useRef, useState } from "react";
import { colors, type Board, type Color, type Game, type Move, type Room, type RoundScore, type Tile } from "./api";
import "./table.css";

const floorCosts = [1, 1, 2, 2, 2, 3, 3];
const sourceName = (source: number) => source === 0 ? "Center" : `Factory ${source}`;

function firstPlayerNote(floorSize: number) {
  if (floorSize >= floorCosts.length) {
    return "Your floor is full. You take the first-player marker at no extra cost and start next round.";
  }
  return `First-player marker on floor space ${floorSize + 1} (−${floorCosts[floorSize]}). You start next round.`;
}

function TilePatterns() {
  return <svg className="tile-patterns" aria-hidden="true" focusable="false">
    <defs>
      <symbol id="motif-blue" viewBox="0 0 32 32"><path fill="currentColor" d="M16 15C8 15 4 11 6 7s8-2 10 6c2-8 8-10 10-6s-2 8-10 8Zm0 2c8 0 12 4 10 8s-8 2-10-6c-2 8-8 10-10 6s2-8 10-8Z"/><circle cx="16" cy="16" r="2.3" fill="currentColor"/></symbol>
      <symbol id="motif-yellow" viewBox="0 0 32 32"><path fill="currentColor" d="m16 3 2.7 7.2 6.8-3.8-3.8 6.8L29 16l-7.3 2.8 3.8 6.8-6.8-3.8L16 29l-2.7-7.2-6.8 3.8 3.8-6.8L3 16l7.3-2.8-3.8-6.8 6.8 3.8Z"/></symbol>
      <symbol id="motif-red" viewBox="0 0 32 32"><path fill="none" stroke="currentColor" strokeWidth="2.2" d="m16 4 12 12-12 12L4 16Z"/><path fill="currentColor" d="m16 10 6 6-6 6-6-6Z"/></symbol>
      <symbol id="motif-black" viewBox="0 0 32 32"><path fill="none" stroke="currentColor" strokeWidth="2.5" d="m5 13 8-8M5 22 17 10m-7 17 12-12m-3 12 8-8M17 5l10 10M8 14l10 10"/></symbol>
      <symbol id="motif-white" viewBox="0 0 32 32"><g fill="none" stroke="currentColor" strokeWidth="2"><circle cx="16" cy="16" r="10"/><circle cx="16" cy="16" r="5"/><path d="M16 2v5m14 9h-5m-9 14v-5M2 16h5"/></g></symbol>
      <symbol id="motif-first" viewBox="0 0 32 32">
        <path d="M4 10V4h6m12 0h6v6m0 12v6h-6m-12 0H4v-6" fill="none" stroke="currentColor" strokeWidth="1.5"/>
        <text x="16" y="24" textAnchor="middle" fill="currentColor" fontFamily="Georgia, serif" fontSize="24" fontWeight="700">1</text>
      </symbol>
    </defs>
  </svg>;
}

function Ceramic({ color, ghost = false, preview = false, pending = false }: { color?: Tile; ghost?: boolean; preview?: boolean; pending?: boolean }) {
  return <span aria-hidden="true" className={`ceramic ${color ?? "blank"}${ghost ? " ghost" : ""}${preview ? " preview" : ""}${pending ? " pending" : ""}`}>
    {color && <svg focusable="false" viewBox="0 0 32 32"><use href={`#motif-${color}`}/></svg>}
  </span>;
}

type Pick = { source: number; color: Color; count: number; revision: number };

function focusControl(control: HTMLButtonElement | null, region: string) {
  if (!control) return;
  control.focus({ preventScroll: true });
  control.closest(region)?.scrollIntoView({ block: "nearest" });
  // On a short landscape screen the whole region may not fit.
  control.scrollIntoView({ block: "nearest" });
}

function Supply({ game, active, selected, onPick }: {
  game: Game; active: boolean; selected: Pick | null; onPick: (pick: Pick, button: HTMLButtonElement) => void;
}) {
  function draftButton(source: number, color: Color, tiles: Color[], index: number) {
    const count = tiles.filter(tile => tile === color).length;
    // Each factory tile is clickable. Matching tiles share one keyboard stop.
    const duplicate = source !== 0 && tiles.indexOf(color) !== index;
    return <button key={source === 0 ? color : index} type="button" className="tile-group" data-source={source} data-color={color}
      aria-label={`Take ${count} ${color} from ${sourceName(source)}`} tabIndex={duplicate ? -1 : undefined}
      aria-pressed={selected?.source === source && selected.color === color}
      disabled={!active} onClick={event => onPick({ source, color, count, revision: game.revision }, event.currentTarget)}>
      <Ceramic color={color} />
      {source === 0 && count > 1 && <span className="tile-quantity" aria-hidden="true">×{count}</span>}
    </button>;
  }
  return <section className="supply" aria-label="Drafting area">
    <div className="factory-rack">{game.factories.map((tiles, index) =>
      <div className="plate" key={index} role="group" aria-label={`${sourceName(index + 1)}${tiles.length ? "" : ", empty"}`}>
        {tiles.map((color, slot) => draftButton(index + 1, color, tiles, slot))}
      </div>
    )}</div>
    <div className="table-center">
      <h3>Center</h3>
      <div className="center-groups">
        {game.markerInCenter && <span className="first-marker" role="img" aria-label="First-player marker" title="The first center draft takes this marker"><Ceramic color="first" /></span>}
        {colors.filter(color => game.center.includes(color)).map(color => draftButton(0, color, game.center, 0))}
        {!game.center.length && !game.markerInCenter && <span className="empty-plate">Empty</span>}
      </div>
    </div>
  </section>;
}

function PlayerBoard({ player, seat, own, name, selected, previewMarker, busy, legalMoves, onPlace, onChangeTiles, previous, round, compact = false }: {
  player: Board; seat: number; own: boolean; name: string; selected: Pick | null; legalMoves: Move[];
  previewMarker: boolean; busy: boolean;
  onPlace: (row: number, clickCount: number) => void; onChangeTiles: () => void; previous?: RoundScore; round?: number; compact?: boolean;
}) {
  const allowed = (row: number) => own && !!selected && legalMoves.some(move =>
    move.source === selected.source && move.color === selected.color && move.row === row);
  return <section className={`player-board${own ? " own" : ""}${compact ? " compact" : ""}`} aria-label={`${name}, seat ${seat}${compact ? ", opponent board" : ""}`}>
    <header className="board-header">
      <div className="board-player">{compact && <span className="opponent-label">Opponent</span>}
        <h2 title={name}>{name} {own && <small>You</small>}</h2>
      </div>
      <p className="points"><strong>{player.score}</strong> pts</p>
    </header>
    {selected && <div className="placement-summary">
      <div id="draft-selection">
        <strong>{selected.count} {selected.color} {selected.count === 1 ? "tile" : "tiles"}</strong> · {sourceName(selected.source)}
        <div role="status">{busy ? "Saving your move…" : "Choose a line or the floor."}</div>
        {previewMarker && <span className="marker-note">{firstPlayerNote(player.floor.length)}</span>}
      </div>
      <button type="button" className="text-button change-tiles" disabled={busy} onClick={onChangeTiles}>Change tiles</button>
    </div>}
    <div className="board-grid">
      {player.lines.map((line, index) => {
        const row = index + 1;
        const enabled = allowed(row);
        const capacity = row - line.count;
        const overflow = enabled ? Math.max(0, selected!.count - capacity) : 0;
        const projected = enabled ? Math.min(capacity, selected!.count) : 0;
        const description = `Pattern line ${row}, ${line.count} of ${row}${line.color ? " " + line.color : ""}`;
        const slots = Array.from({ length: row }, (_, col) => {
          const filled = col >= capacity;
          const preview = !filled && col >= capacity - projected;
          return <Ceramic key={col} color={filled ? line.color || undefined : preview ? selected!.color : undefined} preview={preview} />;
        });
        const wall = player.wall[index];
        return <div key={row} className="board-row">
          {own ? <button type="button" className="pattern-row" data-row={row} disabled={!enabled || busy}
            aria-label={enabled ? `Place on pattern line ${row}, ${line.count} of ${row} filled${overflow ? `, ${overflow} overflow to floor` : ""}` : description}
            aria-describedby={enabled ? "draft-selection" : undefined} onClick={event => onPlace(row, event.detail)}>
            <span className="row-note">{overflow ? `+${overflow} ↓` : row}</span>{slots}
          </button> : <div className="pattern-row" role="img" aria-label={description}><span className="row-note">{row}</span>{slots}</div>}
          <div className="wall-row" role="group" aria-label={`Wall row ${row}`}>
            {wall.map((filled, col) => {
              const color = colors[(col - index + 5) % 5];
              const pending = !filled && line.count === row && line.color === color;
              return <span key={col} role="img" aria-label={`${color}, ${filled ? "filled" : pending ? "pending until end of round" : "empty"}`}
                title={pending ? "Completed pattern line: this tile will move here at the end of the round." : undefined}>
                <Ceramic color={color} ghost={!filled} pending={pending} />
              </span>;
            })}
          </div>
        </div>;
      })}
    </div>
    <div className="floor-caption"><h3 className="board-heading">Floor line</h3>
      {own && selected && <button type="button" className="floor-action" data-row={0} disabled={!allowed(0) || busy}
        aria-describedby="draft-selection" onClick={event => onPlace(0, event.detail)}>Place on floor</button>}
    </div>
    <ol className="floor" aria-label="Floor penalties">{floorCosts.map((cost, index) => {
      const preview = previewMarker && index === player.floor.length;
      const tile = preview ? "first" : player.floor[index];
      const label = tile === "first" ? "first-player marker" : tile ?? "empty";
      return <li key={index}>
        <span className="sr-only">Floor space {index + 1}, {preview && "preview of "}{label}, minus {cost}</span>
        <Ceramic color={tile} preview={preview} /><span aria-hidden="true">−{cost}</span>
      </li>;
    })}</ol>
    {previous && !compact && <p className="round-score">Round {round} <span>+{previous.wall} wall</span><span>−{previous.penalty} floor</span></p>}
  </section>;
}

function Results({ game, name, yourSeat }: { game: Game; name: (seat: number) => string; yourSeat: number }) {
  const tied = game.winners.length === 2;
  const winner = game.winners[0];
  const equalPoints = game.players[0].score === game.players[1].score;
  return <section className="results" aria-label="Game result">
    <h2>{tied ? "Shared victory" : winner === yourSeat ? "You win!" : `${name(winner)} wins`}</h2>
    {equalPoints && <p>{tied ? "Both players tied on points and completed rows." : "Completed horizontal rows decided the tie."}</p>}
    <table><caption className="sr-only">Scoring breakdown</caption><thead><tr><th scope="col">Points</th>
      {[1,2].map(seat => <th key={seat} scope="col">{name(seat)}</th>)}</tr></thead>
      <tbody>{(["Wall & floor", "Row bonuses", "Column bonuses", "Color bonuses", "Total"] as const).map((label, index) =>
        <tr key={label}><th scope="row">{label}</th>{game.players.map((p, seat) => {
          const { rows, columns, colors } = p.bonus;
          return <td key={seat}>{[p.score - rows - columns - colors, rows, columns, colors, p.score][index]}</td>;
        })}</tr>
      )}</tbody>
    </table>
    <a className="another-room" href="/">Create another room</a>
  </section>;
}

export function GameTable({ room, game, busy, onMove }: {
  room: Room; game: Game; busy: boolean; onMove: (move: Move & { revision: number }) => void;
}) {
  const [pick, setPick] = useState<Pick | null>(null);
  const table = useRef<HTMLDivElement>(null);
  const opponent = useRef<HTMLElement>(null);
  const sourceButton = useRef<HTMLButtonElement | null>(null);
  const submittedRevision = useRef<number | null>(null);
  const name = (seat: number) => room.seats.find(p => p.number === seat)?.displayName ?? `Player ${seat}`;
  const yourTurn = !game.finished && game.turn === room.yourSeat;
  const active = yourTurn && !busy;
  const selected = yourTurn && pick?.revision === game.revision ? pick : null;
  const takesMarker = selected?.source === 0 && game.markerInCenter;

  useLayoutEffect(() => {
    const panel = opponent.current;
    if (!panel) return;
    // Keep focused controls below the sticky board, including with larger text.
    const updateHeight = () => table.current?.style.setProperty("--opponent-height", `${panel.getBoundingClientRect().height}px`);
    updateHeight();
    const observer = new ResizeObserver(updateHeight);
    observer.observe(panel);
    return () => observer.disconnect();
  }, [game.finished]);

  useLayoutEffect(() => {
    if (!selected || !table.current) return;
    focusControl(table.current.querySelector<HTMLButtonElement>(".own button[data-row]:enabled"), ".player-board");
  }, [selected]);

  useLayoutEffect(() => {
    if (busy || submittedRevision.current === null || game.revision <= submittedRevision.current) return;
    submittedRevision.current = null;
    // Only hand off focus if the player is still at the placement controls.
    if (document.activeElement !== document.body && !document.activeElement?.matches("[data-row]")) return;
    const next = active ? table.current?.querySelector<HTMLButtonElement>(".supply button:enabled") :
      table.current?.querySelector<HTMLElement>(".game-heading h1");
    next?.focus();
  }, [busy, game.revision, active]);

  function changeTiles() {
    setPick(null);
    submittedRevision.current = null;
    focusControl(sourceButton.current, ".supply");
  }

  function place(row: number, clickCount: number) {
    // A double tap on the supply can land on a row after the board scrolls.
    if (!selected || busy || clickCount > 1) return;
    submittedRevision.current = selected.revision;
    onMove({ source: selected.source, color: selected.color, row, revision: selected.revision });
  }
  const status = game.turn === room.yourSeat ? "Your turn" : `${name(game.turn)}’s turn`;
  const renderBoard = (seat: number, compact = false) =>
    <PlayerBoard key={seat} seat={seat} name={name(seat)} own={seat === room.yourSeat} player={game.players[seat - 1]}
      selected={seat === room.yourSeat ? selected : null} previewMarker={seat === room.yourSeat && takesMarker} legalMoves={game.legalMoves}
      busy={busy} onPlace={place} onChangeTiles={changeTiles} previous={game.lastRound?.players[seat - 1]} round={game.lastRound?.round} compact={compact} />;
  return <div ref={table} className="game-table" data-game-revision={game.revision} data-turn={game.turn}
    onKeyDown={event => {
      // Holding Enter to pick must not also activate the newly focused destination.
      if (event.key === "Enter" && event.repeat) event.preventDefault();
    }}>
    <TilePatterns />
    <div className="game-heading"><h1 tabIndex={-1}>{game.finished ? "Final scores" : `Round ${game.round}`}</h1>
      {!game.finished && <p className="turn" role="status"><i className={game.turn === room.yourSeat ? "active" : ""} />{status}</p>}
    </div>
    <div className={game.finished ? "finished-table" : "match-layout"}>
      {!game.finished && <aside ref={opponent} className="opponent-overview" aria-label="Opponent overview">
        {renderBoard(3 - room.yourSeat, true)}
      </aside>}
      <div className="match-main">
        {game.finished ? <Results game={game} name={name} yourSeat={room.yourSeat} /> : <>
          <Supply game={game} active={active} selected={selected} onPick={(nextPick, button) => {
            sourceButton.current = button;
            setPick(nextPick);
          }} />
          <div className="draft-help" role="status">{active && !selected ? "Choose a tile color." : null}</div>
        </>}
        {game.lastMove && <p className="last-draft" role="status"><strong>Last draft</strong> {name(game.lastMove.seat)} took {game.lastMove.count} {game.lastMove.color} from {game.lastMove.source === 0 ? "the center" : sourceName(game.lastMove.source)} for {game.lastMove.row ? `line ${game.lastMove.row}` : "the floor"}.</p>}
        <div className="boards">{(game.finished ? [room.yourSeat, 3 - room.yourSeat] : [room.yourSeat]).map(seat => renderBoard(seat))}</div>
      </div>
    </div>
  </div>;
}

export function PlayGuide() {
  return <details className="play-guide"><summary>How to play</summary>
    <ol>
      <li>Take every tile of one color from a factory or the center. A factory’s other tiles move to the center. The first center draft also puts the start marker on your floor and gives you first turn next round.</li>
      <li>Choose one pattern line or the floor. A line holds one color and its wall row must still have room for that color. Extra tiles go to the floor.</li>
      <li>When the displays empty, each full line adds one tile to your wall. An isolated tile scores one point. Otherwise, score the length of each connected horizontal and vertical run through it. Floor spaces deduct the printed points.</li>
      <li>A complete wall row ends the game after scoring that round. Each full row earns 2 bonus points, each full column 7, and five tiles of one color 10. Tied scores go to the player with more complete rows, then to a shared win.</li>
    </ol>
    <a href="https://cdn.svc.asmodee.net/production-nextmove/uploads/sites/4/2024/06/EN-Azul-Rules-Next-Move-web.pdf" target="_blank" rel="noreferrer">Publisher’s rulebook</a>
  </details>;
}
