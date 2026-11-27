import { useId, useRef, useState } from "react";
import { colors, type Board, type HistoricalTable, type HistoryEvent, type MatchHistory, type Tile } from "./api";
import { Ceramic } from "./Ceramic";
import { historyDraftEffects } from "./historyDraft";
import { TableIcon } from "./TableActions";
import "./history-board.css";

type ViewerProps = {
  history: MatchHistory;
  names: [string, string];
  initialIndex: number;
  full: boolean;
  onBack: () => void;
};

const floorCosts = [1, 1, 2, 2, 2, 3, 3];

function describeEvent(event: HistoryEvent, previous: HistoricalTable | undefined, names: [string, string]) {
  const table = event.table;
  const move = table.lastMove;
  if (event.kind === "draft" && move) {
    const source = move.source === 0 ? "Center" : `Factory ${move.source}`;
    const parts = [`${source} → ${move.row ? `Line ${move.row}` : "Floor"}`];
    parts.push(...historyDraftEffects(table, previous));
    return { title: `${names[move.seat - 1]} took ${move.count} ${move.color}`, detail: parts.join(" · "), color: move.color };
  }
  switch (event.kind) {
    case "opening": return { title: "Opening deal", detail: `${names[table.turn - 1]} drafts first.` };
    case "refill": return { title: "Factories refilled", detail: `${names[table.turn - 1]} starts round ${table.round}.` };
    case "scoring": return { title: `Round ${table.round} scored`, detail: "Completed lines moved to the wall and floor penalties were applied." };
    case "final": {
      const tied = table.winners.length === 2;
      const equal = table.players[0].score === table.players[1].score;
      const title = tied ? "Shared victory" : `${names[table.winners[0] - 1]} wins`;
      const detail = tied ? "Both players tied on points and completed rows." : equal ? "Completed rows decided the tie." : "Final wall bonuses applied.";
      return { title, detail };
    }
    default: return { title: table.finished ? "Saved final board" : "Recorded position", detail: previous ? "Recording resumed here after a gap." : "The earlier moves are unavailable." };
  }
}

function BoardTile({ color, label, ghost, added }: { color?: Tile; label: string; ghost?: boolean; added?: boolean }) {
  const description = added ? label + ", added at this step" : label;
  return <span className="history-piece" data-added={added || undefined} role="img" aria-label={description} title={description}>
    <Ceramic color={color} ghost={ghost} />
  </span>;
}

function RecordedBoard({ board, previous }: { board: Board; previous?: Board }) {
  return <>
    <div className="history-mosaic">
      <span className="history-board-label">Pattern lines</span><span className="history-board-label">Wall</span>
      {board.lines.map((line, row) => {
        const length = row + 1;
        const oldLine = previous?.lines[row];
        const oldCount = oldLine?.color === line.color ? oldLine.count : 0;
        return <div className="history-board-row" key={row}>
          <div className="history-pattern">
            <span className="history-line-number" aria-hidden="true">{length}</span>
            {Array.from({ length }, (_, i) => {
              const color = i >= length - line.count && line.color ? line.color : undefined;
              return <BoardTile key={i} color={color} label={`Line ${length}, space ${i + 1}, ${color || "empty"}`}
                added={!!previous && !!color && i < length - oldCount} />;
            })}
          </div>
          <div className="history-wall">
            {board.wall[row].map((filled, column) => {
              const color = colors[(column - row + colors.length) % colors.length];
              return <BoardTile key={column} color={color} ghost={!filled} label={`Wall row ${length}, ${color}, ${filled ? "filled" : "empty"}`}
                added={!!previous && filled && !previous.wall[row][column]} />;
            })}
          </div>
        </div>;
      })}
    </div>
    <div className="history-floor"><span className="history-board-label">Floor</span><div>
      {floorCosts.map((cost, i) => {
        const color = board.floor[i];
        return <BoardTile key={i} color={color} label={`Floor space ${i + 1}, ${color === "first" ? "first-player marker" : color || "empty"}, minus ${cost}`}
          added={!!previous && !!color && i >= previous.floor.length} />;
      })}
    </div></div>
  </>;
}

function RecordedSupply({ table, expanded }: { table: HistoricalTable; expanded: boolean }) {
  const total = table.factories.reduce((sum, group) => sum + group.length, table.center.length);
  return <details className="history-supply" open={expanded}>
    <summary><strong>Tile supply</strong><span>{total} {total === 1 ? "tile" : "tiles"} <span aria-hidden="true">⌄</span></span></summary>
    <div className="history-factories">{table.factories.map((factory, i) =>
      <div key={i} className="history-factory" role="img" aria-label={`Factory ${i + 1}, ${factory.join(", ") || "empty"}`}>
        <div className="history-plate">{factory.map((color, slot) => <Ceramic key={slot} color={color} />)}</div><span aria-hidden="true">F{i + 1}</span>
      </div>)}</div>
    <div className="history-center"><strong>Center</strong>
      {table.markerInCenter && <span role="img" aria-label="First-player marker"><Ceramic color="first" /></span>}
      {colors.map(color => {
        const count = table.center.filter(tile => tile === color).length;
        return count > 0 && <span role="img" aria-label={`${count} ${color}`} key={color}><Ceramic color={color} /><span aria-hidden="true">×{count}</span></span>;
      })}
      {!table.markerInCenter && !table.center.length && <span>Empty</span>}
    </div>
  </details>;
}

function BoardNote({ event, seat, previous }: { event: HistoryEvent; seat: number; previous?: Board }) {
  const board = event.table.players[seat - 1];
  const move = event.table.lastMove;
  const round = event.table.lastRound?.players[seat - 1];
  let change = "", detail = "";
  if (event.kind === "draft" && move?.seat === seat && previous) {
    const row = move.row;
    if (board.floor.length > previous.floor.length || (row > 0 && board.lines[row - 1].count > previous.lines[row - 1].count)) change = "Added this move";
    if (row) detail = `Line ${row} holds ${board.lines[row - 1].count} of ${row}`;
  } else if (event.kind === "scoring" && round) {
    if (round.wall && previous) change = "Placed on wall";
    detail = `+${round.wall} wall · −${round.penalty} floor`;
  } else if (event.kind === "final") {
    const b = board.bonus;
    detail = `+${b.rows} rows · +${b.columns} columns · +${b.colors} colors`;
  }
  return change || detail ? <div className="history-board-note">
    {change && <span className="history-change-key">{change}</span>}{detail && <span>{detail}</span>}
  </div> : null;
}

function RecordedPosition({ history, names, initialIndex, full, onBack }: ViewerProps) {
  const [position, setPosition] = useState(initialIndex);
  const [chosenSeat, setChosenSeat] = useState<number | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const list = history.events;
  const index = Math.max(0, Math.min(position, list.length - 1));
  const event = list[index];
  const previous = list[index - 1]?.table;
  const finished = list.at(-1)?.table.finished;
  const title = full ? "Match replay" : "Board snapshot";
  const back = <button type="button" className="history-board-back" data-history-back onClick={onBack}><TableIcon name="back" />{full ? "Final scores" : "Move history"}</button>;
  if (!event) return <section className="history-board-view" data-history-board>{back}<p>No positions have been recorded.</p></section>;

  const table = event.table;
  const seat = chosenSeat ?? (event.kind === "draft" && table.lastMove ? table.lastMove.seat : table.winners[0] || table.turn || table.lastMove?.seat || 1);
  const description = describeEvent(event, previous, names);
  const moveNo = list.slice(0, index + 1).filter(item => item.kind === "draft").length;
  const moveCount = list.filter(item => item.kind === "draft").length;
  const moveCaption = history.complete ? "After move" : "After recorded move";
  const comparedBoard = event.kind === "draft" || event.kind === "scoring" ? previous?.players[seat - 1] : undefined;
  const stepCaption = `Step ${index + 1} of ${list.length}`;
  function seek(next: number) {
    setPosition(next);
    viewport.current?.scrollTo({ top: 0 });
  }

  return <section className="history-board-view" data-history-board data-history-index={index} data-history-kind={event.kind} aria-labelledby={titleId}>
    <nav className="history-board-nav" aria-label="Match review">{back}<span>Read-only</span></nav>
    <div className="history-board-scroll" tabIndex={0} ref={viewport} role="region" aria-label="Recorded position">
      <header className="history-board-title"><h2 id={titleId}>{title}</h2><p>{finished ? "Completed match" : "Match in progress"}</p></header>
      {!history.complete && event.kind !== "baseline" && <p className="history-partial">Only recorded moves are numbered.</p>}
      <div className="history-draft-caption">{description.color && <span className="history-draft-tile"><Ceramic color={description.color} /></span>}
        <div><p className="history-event-label">Round {table.round}{event.kind === "draft" && <> · {moveCaption} {moveNo} of {moveCount}</>}</p>
          <h3>{description.title}</h3><p>{description.detail}</p></div>
      </div>
      <section className="history-position" aria-label={`${names[seat - 1]}’s recorded board`}>
        <div className="history-board-choices" role="group" aria-label="Choose a player’s board">
          {table.players.map((player, i) => <button key={i} type="button" data-replay-seat={i + 1} aria-pressed={seat === i + 1}
            title={names[i]} aria-label={`${names[i]}, seat ${i + 1}, ${player.score} points`} onClick={() => setChosenSeat(i + 1)}>
            <span>{names[i]}</span><span className="history-score"><b>{player.score}</b> pts</span></button>)}
        </div>
        <RecordedBoard board={table.players[seat - 1]} previous={comparedBoard} />
        <BoardNote event={event} seat={seat} previous={comparedBoard} />
      </section>
      <RecordedSupply key={index} table={table} expanded={!full || event.kind === "opening" || event.kind === "refill"} />
    </div>
    {full && <div className="history-transport" role="group" aria-label="Replay controls">
      <label><span>{stepCaption}</span><input data-replay-range type="range" min={0} max={list.length - 1} step={1} value={index} disabled={list.length < 2}
        aria-label="Choose a recorded step" aria-valuetext={`${stepCaption}. ${description.title}`} onChange={e => seek(Number(e.target.value))} /></label>
      <div><button type="button" className="secondary" data-replay-step="-1" disabled={index === 0} onClick={() => seek(index - 1)}>
        <TableIcon name="back" />Previous</button><button type="button" className="secondary" data-replay-step="1" disabled={index === list.length - 1}
        onClick={() => seek(index + 1)}>Next<TableIcon name="forward" /></button></div>
    </div>}
    <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">{description.title}. {names[seat - 1]}’s board, {table.players[seat - 1].score} points.</span>
  </section>;
}

export function HistoryBoard(props: ViewerProps) {
  return <RecordedPosition key={props.history.matchId} {...props} />;
}
