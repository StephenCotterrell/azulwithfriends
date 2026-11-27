import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { colors, seatName, type Board, type Color, type Game, type Move, type Room, type RoundScore } from "./api";
import { Ceramic } from "./Ceramic";
import { MatchReviewDialog, HistoryStatus, useMatchHistory } from "./MatchReview";
import { ScoreHistory } from "./ScoreHistory";
import "./table.css";
import { TableIcon } from "./TableActions";
import "./match-controls.css";

const placementPrompt = "Choose a pattern line or the floor";
const floorCosts = [1, 1, 2, 2, 2, 3, 3];
const sourceName = (source: number) => source === 0 ? "Center" : `Factory ${source}`;

function firstPlayerNote(floorSize: number) {
  if (floorSize >= floorCosts.length) {
    return "Free marker. You start next round.";
  }
  return `Marker −${floorCosts[floorSize]}. You start next round.`;
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

type Pick = { source: number; color: Color; count: number; revision: number; keyboard: boolean };

function focusControl(control: HTMLElement | null | undefined, keyboard: boolean) {
  if (!control) return;
  control.focus({ preventScroll: true });
  if (keyboard) control.scrollIntoView({ block: "nearest", inline: "nearest" });
}

function PickSummary({ count, color, source, markerNote }: {
  count: number; color: Color; source: number; markerNote: string;
}) {
  return <div className="picked-info">
    <span className="picked-glaze"><Ceramic color={color} /></span>
    <span>
      <strong>{count} {color} {count === 1 ? "tile" : "tiles"} <b aria-hidden="true">·</b> {sourceName(source)}</strong>
      {markerNote && <small>{markerNote}</small>}
    </span>
  </div>;
}

function Supply({ game, active, selected, onPick, showHint, markerNote, onChange }: {
  showHint: boolean; markerNote: string; onChange: (keyboard: boolean) => void;
  game: Game; active: boolean; selected: Pick | null; onPick: (pick: Pick, button: HTMLButtonElement) => void;
}) {
  function draftButton(source: number, color: Color, tiles: Color[], index: number) {
    const count = tiles.filter(tile => tile === color).length;
    // Each factory tile is clickable. Matching tiles share one keyboard stop.
    const duplicate = source !== 0 && tiles.indexOf(color) !== index;
    return <button key={source === 0 ? color : index} type="button" className="tile-group" data-source={source} data-color={color}
      aria-label={`Take ${count} ${color} from ${sourceName(source)}`} tabIndex={duplicate ? -1 : undefined}
      aria-pressed={selected?.source === source && selected.color === color}
      disabled={!active} onClick={event => onPick({ source, color, count, revision: game.revision, keyboard: event.detail === 0 }, event.currentTarget)}>
      <Ceramic color={color} />
      {source === 0 && count > 1 && <span className="tile-quantity" aria-hidden="true">×{count}</span>}
    </button>;
  }
  return <section className="supply" aria-label="Drafting area">
    <div className={`supply-bar ${selected ? "supply-summary" : showHint ? "supply-hint" : "supply-title"}`}>
      <div className="supply-copy">
        {/* Size the shared track to the longest draft label at the player's text size. */}
        <div className="draft-sizer" aria-hidden="true">
          <PickSummary count={20} color="yellow" source={0} markerNote={firstPlayerNote(floorCosts.length)} />
        </div>
        {selected ? <PickSummary {...selected} markerNote={markerNote} /> :
          <div className="supply-heading">{showHint ? <><TableIcon name="pick" />Pick a tile color</> : "Tile supply"}</div>}
      </div>
      <div className="supply-action">
        <small className={showHint && !selected ? undefined : "draft-sizer"}>Factories or center</small>
        <button type="button" className={`text-button change-draft${selected ? "" : " draft-sizer"}`}
          disabled={!active || !selected} aria-hidden={!selected || undefined}
          onClick={event => onChange(event.detail === 0)}>Change tiles</button>
      </div>
    </div>
    <div className="factory-rack">{game.factories.map((tiles, index) =>
      <div className="plate" key={index} role="group" aria-label={`${sourceName(index + 1)}${tiles.length ? "" : ", empty"}`}>
        {tiles.map((color, slot) => draftButton(index + 1, color, tiles, slot))}
      </div>
    )}</div>
    <div className="table-center">
      <h3>Center</h3>
      <div className="center-groups">
        <span className={`first-marker${game.markerInCenter ? "" : " draft-sizer"}`} aria-hidden={!game.markerInCenter || undefined}
          role="img" aria-label="First-player marker" title="The first center draft takes this marker"><Ceramic color="first" /></span>
        {colors.map(color => game.center.includes(color) ? draftButton(0, color, game.center, 0) :
          <span key={color} className="tile-group draft-sizer" aria-hidden="true"><Ceramic color={color} /></span>)}
        {!game.center.length && !game.markerInCenter && <span className="empty-plate">Empty</span>}
      </div>
    </div>
  </section>;
}

function PlayerBoard({ player, anticipatedScore, seat, own, name, selected, previewMarker, busy, legalMoves, onPlace, instruction, saving, previous, round, compact = false, alignedFloor = false }: {
  player: Board; anticipatedScore?: number; seat: number; own: boolean; name: string; selected: Pick | null; legalMoves: Move[];
  previewMarker: boolean; busy: boolean;
  onPlace: (row: number, clickCount: number) => void; instruction?: string; saving: boolean; previous?: RoundScore; round?: number; compact?: boolean; alignedFloor?: boolean;
}) {
  const allowed = (row: number) => own && !!selected && legalMoves.some(move =>
    move.source === selected.source && move.color === selected.color && move.row === row);
  const savingSelection = !!selected && saving;
  const floorHeading = <h3 className="board-heading">Floor</h3>;
  const floor = <ol className="floor" aria-label="Floor penalties">{floorCosts.map((cost, index) => {
      const preview = previewMarker && index === player.floor.length;
      const tile = preview ? "first" : player.floor[index];
      const label = tile === "first" ? "first-player marker" : tile ?? "empty";
      return <li key={index}>
        <span className="sr-only">Floor space {index + 1}, {preview && "preview of "}{label}, minus {cost}</span>
        <Ceramic color={tile} preview={preview} /><span className="floor-cost" aria-hidden="true">−<wbr />{cost}</span>
      </li>;
    })}</ol>;
  return <section className={`player-board${own ? " own" : ""}${compact ? " compact" : ""}`} aria-label={`${name}, seat ${seat}${compact ? ", opponent board" : ""}`}>
    <header className="board-header">
      <div className="board-player">{compact && <span className="opponent-label">Opponent</span>}
        <h2 title={name}>{name} {own && <small>You</small>}</h2>
      </div>
      <p className="points"><strong>{player.score}</strong><span className="sr-only"> points</span>{anticipatedScore !== undefined && <>{" "}
        <strong className="score-preview" title="Current score plus bonuses already earned on the wall" aria-hidden="true">({anticipatedScore})</strong>
        <span className="sr-only">, {anticipatedScore} points including current wall bonuses</span>
      </>}<span aria-hidden="true"> pts</span></p>
    </header>
    {alignedFloor && <div className="board-caption">
      <span className="board-prompt draft-sizer" aria-hidden="true"><TableIcon name="place" />{placementPrompt}</span>
      <p className={`board-message ${savingSelection ? "board-progress" : "draft-sizer"}`} aria-hidden={!savingSelection}>
        <span className="saving-mark" aria-hidden="true" />Saving your move…
      </p>
      {!savingSelection && (selected && instruction
        ? <div className="board-prompt board-hint"><TableIcon name="place" />{instruction}</div>
        : <div className="board-columns"><span>Pattern lines</span><span>Wall</span></div>)}
    </div>}
    <div className={`board-grid${alignedFloor ? " aligned-floor-grid" : ""}`}>
      {player.lines.map((line, index) => {
        const row = index + 1;
        const enabled = allowed(row);
        const capacity = row - line.count;
        const overflow = enabled ? Math.max(0, selected!.count - capacity) : 0;
        const floorLabel = overflow ? `${overflow} ${overflow === 1 ? "tile" : "tiles"} to floor` : "";
        const projected = enabled ? Math.min(capacity, selected!.count) : 0;
        const blocked = !selected || enabled ? "" : line.count >= row ? "line full" :
          line.count && line.color !== selected.color ? `line already holds ${line.color}` : `${selected.color} already on this wall row`;
        const description = `Pattern line ${row}, ${line.count} of ${row}${line.color ? " " + line.color : ""}`;
        const slots = Array.from({ length: row }, (_, col) => {
          const filled = col >= capacity;
          const preview = !filled && col >= capacity - projected;
          return <Ceramic key={col} color={filled ? line.color || undefined : preview ? selected!.color : undefined} preview={preview} />;
        });
        const wall = player.wall[index];
        return <div key={row} className="board-row">
          {own ? <button type="button" className="pattern-row" data-row={row} data-legal={enabled ? "yes" : undefined} disabled={!enabled || busy}
            aria-label={enabled ? `Place on pattern line ${row}, ${line.count} of ${row} filled${floorLabel ? `, ${floorLabel}` : ""}` : description + (blocked ? `, unavailable, ${blocked}` : "")}
            aria-describedby={enabled ? "draft-selection" : undefined} onClick={event => onPlace(row, event.detail)}>
            <span className="line-label" aria-hidden="true">
              <span className="line-number">{row}</span>
              {overflow > 0 && <span className="overflow-note" title={floorLabel}>
                <span>{overflow}</span>{" "}<span><span className="overflow-to">to </span>floor</span>
              </span>}
            </span>
            <span className="pattern-tiles">{slots}</span>
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
      {alignedFloor && <>
        <div className="aligned-floor-strip">{floorHeading}{floor}</div>
        <button type="button" className={`floor-action aligned-floor-action${selected ? "" : " draft-sizer"}`}
          data-row={0} disabled={!allowed(0) || busy} aria-hidden={!selected || undefined}
          aria-describedby={selected ? "draft-selection" : undefined} onClick={event => onPlace(0, event.detail)}>Place on floor</button>
      </>}
    </div>
    {!alignedFloor && <><div className="floor-caption">{floorHeading}</div>{floor}</>}
    {previous && !compact && <p className="round-score">Round {round} <span>+{previous.wall} wall</span><span>−{previous.penalty} floor</span></p>}
  </section>;
}

function DraftRoute({ move, actor }: { move: Move & { count: number }; actor: string }) {
  return <>
    <span className="recap-actor"><b>Last</b><span className="recap-dot">·</span>{actor}</span>
    <span className="recap-piece"><Ceramic color={move.color} /><b>{move.count} {move.color}</b></span>
    <span className="recap-route"><span className="recap-dot">·</span>{sourceName(move.source)} <span className="route-arrow">→</span> {move.row ? `Line ${move.row}` : "Floor"}</span>
  </>;
}

function LastDraft({ move, actor, opponent }: { move: Game["lastMove"]; actor: string; opponent?: string }) {
  return <p className="compact-recap" role="status" aria-atomic="true">
    {opponent && <span className="recap-visible draft-sizer" aria-hidden="true">
      <DraftRoute actor={opponent.length < 3 ? "You" : opponent} move={{ source: 5, color: "yellow", count: 20, row: 5 }} />
    </span>}
    {move ? <>
      <span className="sr-only">Last draft. {actor} took {move.count} {move.color} {move.count === 1 ? "tile" : "tiles"} from {sourceName(move.source)} for {move.row ? `line ${move.row}` : "the floor"}.</span>
      <span className="recap-visible" aria-hidden="true"><DraftRoute move={move} actor={actor} /></span>
    </> : <span>Opening draft</span>}
  </p>;
}

type Rematch = { pending: boolean; onStart: () => void };

function Results({ game, name, yourSeat, busy, rematch, children }: {
  game: Game; name: (seat: number) => string; yourSeat: number; busy: boolean; rematch?: Rematch; children: ReactNode;
}) {
  const tied = game.winners.length === 2;
  const winner = game.winners[0];
  const equalPoints = game.players[0].score === game.players[1].score;
  return <section className="results" aria-label="Game result">
    <h2>{tied ? "Shared victory" : winner === yourSeat ? "You win!" : `${name(winner)} wins`}</h2>
    {equalPoints && <p>{tied ? "Both players tied on points and completed rows." : "Completed horizontal rows decided the tie."}</p>}
    {children}
    <table><caption className="sr-only">Scoring breakdown</caption><thead><tr><th scope="col">Points</th>
      {[1,2].map(seat => <th key={seat} scope="col"><span title={name(seat)}>{name(seat)}</span></th>)}</tr></thead>
      <tbody>{(["Wall & floor", "Row bonuses", "Column bonuses", "Color bonuses", "Total"] as const).map((label, index) =>
        <tr key={label}><th scope="row">{label}</th>{game.players.map((p, seat) => {
          const { rows, columns, colors } = p.bonus;
          return <td key={seat}>{[p.score - rows - columns - colors, rows, columns, colors, p.score][index]}</td>;
        })}</tr>
      )}</tbody>
    </table>
    <div className="actions">
      {rematch && <button type="button" disabled={busy} onClick={rematch.onStart}>{rematch.pending ? "Starting rematch…" : "Rematch"}</button>}
      <a href="/">Create another room</a>
    </div>
  </section>;
}

export function GameTable({ room, game, busy, savingMove, networkPaused, draftHints, onMove, onRefresh, rematch }: {
  room: Room; game: Game; busy: boolean; savingMove: boolean; networkPaused: boolean; draftHints: boolean; onRefresh: () => void; onMove: (move: Move & { revision: number }) => void; rematch?: Rematch;
}) {
  const [review, setReview] = useState<{ matchId: string; revision: number; full: boolean; trigger: HTMLButtonElement } | null>(null);
  const reviewOpen = !!review && review.matchId === game.matchId;
  const reader = reviewOpen ? "review" : game.finished ? "results" : "closed";
  const { history, error, retry } = useMatchHistory(room.id, game.matchId, game.revision, reader, onRefresh);
  const resultHistory = history?.revision === game.revision ? history : null;
  const [pick, setPick] = useState<Pick | null>(null);
  const table = useRef<HTMLDivElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const opponent = useRef<HTMLElement>(null);
  const sourceButton = useRef<HTMLButtonElement | null>(null);
  const submission = useRef<{ revision: number; keyboard: boolean } | null>(null);
  const wasFinished = useRef(game.finished);
  const name = (number: number) => {
    const seat = room.seats.find(player => player.number === number);
    return seat ? seatName(seat) : `Player ${number}`;
  };
  const yourTurn = !game.finished && game.turn === room.yourSeat;
  const locked = busy || networkPaused;
  const active = yourTurn && !locked;
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
    focusControl(table.current.querySelector<HTMLButtonElement>(".own button[data-row]:enabled"), selected.keyboard);
  }, [selected]);

  useLayoutEffect(() => {
    if (wasFinished.current && !game.finished && document.activeElement === document.body) heading.current?.focus();
    wasFinished.current = game.finished;
  }, [game.finished]);

  useEffect(() => {
    if (!selected || locked) return;
    function handleClickAway(event: MouseEvent) {
      if (event.target instanceof Element && (event.target.closest("[data-keep-draft], dialog") || table.current?.contains(event.target.closest("button")))) return;
      clearPick();
    }
    document.addEventListener("click", handleClickAway, true);
    return () => document.removeEventListener("click", handleClickAway, true);
  }, [selected, locked]);

  useLayoutEffect(() => {
    const submitted = submission.current;
    if (busy || !submitted || game.revision <= submitted.revision) return;
    submission.current = null;
    // Only hand off focus if the player is still at the placement controls.
    if (document.activeElement !== document.body && !document.activeElement?.matches("[data-row]")) return;
    const next = active ? table.current?.querySelector<HTMLButtonElement>(".supply button:enabled") :
      heading.current;
    focusControl(next, submitted.keyboard);
  }, [busy, game.revision, active]);

  function clearPick() {
    setPick(null);
    submission.current = null;
  }

  function changeTiles(keyboard: boolean) {
    clearPick();
    focusControl(sourceButton.current, keyboard);
  }

  function place(row: number, clickCount: number) {
    // Ignore the second click of a double tap while handing off to a destination.
    if (!selected || locked || clickCount > 1) return;
    submission.current = { revision: selected.revision, keyboard: clickCount === 0 };
    onMove({ source: selected.source, color: selected.color, row, revision: selected.revision });
  }
  const seatLabel = (seat: number) => room.seats.find(s => s.number === seat)?.displayName || name(seat);
  const names: [string, string] = [seatLabel(1), seatLabel(2)];
  function openReview(trigger: HTMLButtonElement, full: boolean) {
    if (game.matchId) setReview({ matchId: game.matchId, revision: game.revision, trigger, full });
  }
  const status = yourTurn ? "Your turn" : `${seatLabel(game.turn)}’s turn`;
  const move = game.lastMove;
  const actor = move && move.seat !== room.yourSeat ? seatLabel(move.seat) : "You";
  const lastDraft = <LastDraft move={move} actor={actor} opponent={game.finished ? undefined : seatLabel(3 - room.yourSeat)} />;
  const rows = selected ? game.legalMoves
    .filter(move => move.source === selected.source && move.color === selected.color && move.row > 0)
    .map(move => move.row) : [];
  const markerNote = takesMarker ? firstPlayerNote(game.players[room.yourSeat - 1].floor.length) : "";
  const instruction = draftHints && active
    ? (rows.length ? placementPrompt : "Place these tiles on the floor")
    : undefined;
  let selectionStatus = active && draftHints ? "Pick a tile color from one factory or the center." : "";
  if (selected) {
    const available = rows.length
      ? `Pattern ${rows.length === 1 ? "line" : "lines"} ${rows.join(", ")} and the floor are available.`
      : "Only the floor is available.";
    const next = savingMove ? "Saving your move." : networkPaused ? "Moves paused." : available;
    selectionStatus = [
      `${selected.count} ${selected.color} ${selected.count === 1 ? "tile" : "tiles"} from ${sourceName(selected.source)}.`,
      markerNote, instruction && instruction + ".", next,
    ].filter(Boolean).join(" ");
  }
  const renderBoard = (seat: number, compact = false) =>
    <PlayerBoard key={seat} seat={seat} name={name(seat)} own={seat === room.yourSeat} player={game.players[seat - 1]}
      anticipatedScore={game.finished ? undefined : game.anticipatedScores?.[seat - 1]} alignedFloor={!game.finished && seat === room.yourSeat}
      selected={seat === room.yourSeat ? selected : null} previewMarker={seat === room.yourSeat && takesMarker} legalMoves={game.legalMoves}
      busy={locked} saving={savingMove} instruction={instruction} onPlace={place} previous={game.lastRound?.players[seat - 1]} round={game.lastRound?.round} compact={compact} />;
  return <div ref={table} className="game-table" data-game-revision={game.revision} data-turn={game.turn} data-hints={draftHints ? "on" : "off"}
    onKeyDown={event => {
      // Holding Enter to pick must not also activate the newly focused destination.
      if (event.key === "Enter" && event.repeat) event.preventDefault();
    }}>
    <TilePatterns />
    <div className="game-heading"><h1 ref={heading} tabIndex={-1}>{game.finished ? "Final scores" : `Round ${game.round}`}</h1>
      {!game.finished && <p className="turn" role="status"><i className={game.turn === room.yourSeat ? "active" : ""} /><span title={status}>{status}</span></p>}
    </div>
    {!game.finished && <div className="history-recap" data-keep-draft>
      {lastDraft}
      {game.matchId && <button type="button" aria-haspopup="dialog" className="text-button history-entry"
        onClick={event => openReview(event.currentTarget, false)}><TableIcon name="history" />History</button>}
    </div>}
    <span id="draft-selection" className="sr-only" role="status" aria-live="polite" aria-atomic="true">{selectionStatus}</span>
    <div className={game.finished ? "finished-table" : "match-layout"}>
      {!game.finished && <aside ref={opponent} className="opponent-overview" aria-label="Opponent overview">
        {renderBoard(3 - room.yourSeat, true)}
      </aside>}
      <div className="match-main">
        {game.finished ? <Results game={game} name={name} yourSeat={room.yourSeat} busy={locked} rematch={rematch}>
          {resultHistory && <ScoreHistory history={resultHistory} names={names} yourSeat={room.yourSeat} />}
          {game.matchId && <HistoryStatus pending={!resultHistory} error={error} retry={retry} />}
          {resultHistory && <button type="button" className="open-replay" aria-haspopup="dialog"
            onClick={event => openReview(event.currentTarget, true)}><TableIcon name="history" />Replay match</button>}
        </Results> :
          <Supply game={game} active={active} selected={selected} showHint={active && draftHints} markerNote={markerNote} onChange={changeTiles} onPick={(nextPick, button) => {
            sourceButton.current = button;
            setPick(nextPick);
          }} />}
        {game.finished && lastDraft}
        <div className="boards">{(game.finished ? [room.yourSeat, 3 - room.yourSeat] : [room.yourSeat]).map(seat => renderBoard(seat))}</div>
      </div>
    </div>
    {reviewOpen && <MatchReviewDialog key={review.matchId} full={review.full} trigger={review.trigger} fallback={heading}
      history={history && (history.revision >= review.revision || error) ? history : null} error={error} retry={retry} names={names} yourSeat={room.yourSeat}
      returnLabel={game.finished ? "Return to final scores" : yourTurn ? "Return to draft" : "Return to table"}
      interrupted={networkPaused} onClose={() => setReview(null)} />}
  </div>;
}

export function PlayGuide() {
  return <div className="play-guide">
      <ol>
        <li>Take every tile of one color from a factory or the center. A factory’s other tiles move to the center. The first center draft also puts the start marker on your floor and gives you first turn next round.</li>
        <li>Choose one pattern line or the floor. A line holds one color and its wall row must still have room for that color. Extra tiles go to the floor.</li>
        <li>When the displays empty, each full line adds one tile to your wall. An isolated tile scores one point. Otherwise, score the length of each connected horizontal and vertical run through it. Floor spaces deduct the printed points.</li>
        <li>A complete wall row ends the game after scoring that round. Each full row earns 2 bonus points, each full column 7, and five tiles of one color 10. Tied scores go to the player with more complete rows, then to a shared win.</li>
        <li>The score in brackets includes end-game bonuses from tiles already on your wall, but not pending placements or floor penalties.</li>
      </ol>
      <a href="https://cdn.svc.asmodee.net/production-nextmove/uploads/sites/4/2024/06/EN-Azul-Rules-Next-Move-web.pdf" target="_blank" rel="noreferrer">Publisher’s rulebook</a>
  </div>;
}
