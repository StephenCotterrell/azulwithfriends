import { Ceramic } from "./Ceramic";
import { TableIcon } from "./TableActions";
import type { HistoricalTable, HistoryEvent, MatchHistory } from "./api";
import { historyDraftEffects } from "./historyDraft";
import "./move-history-sheet.css";

type Entry = { event: HistoryEvent; index: number; moveNumber: number };
type Round = { number: number; entries: Entry[]; scored: boolean };
type PlayerLabel = (seat: number) => string;

function signed(points: number) {
  return points < 0 ? `−${-points}` : points > 0 ? `+${points}` : "0";
}

function RoundScores({ table, before, player }: {
  table: HistoricalTable; before?: HistoricalTable; player: PlayerLabel;
}) {
  const result = table.lastRound;
  if (!result || result.round !== table.round) return null;
  return <table className="history-round-scores">
    <caption className="sr-only">Round {result.round} score changes</caption>
    <thead><tr>{["Player", "Wall", "Floor", "Change", "Total"].map(label =>
      <th scope="col" key={label}>{label}</th>)}</tr></thead>
    <tbody>{result.players.map((points, i) => {
      const total = table.players[i].score;
      // The engine records the applied floor penalty, including the zero-score limit.
      const change = before ? total - before.players[i].score : points.wall - points.penalty;
      return <tr key={i}><th scope="row">{player(i + 1)}</th><td>{signed(points.wall)}</td>
        <td>{signed(-points.penalty)}</td><td className="history-score-change">{signed(change)}</td>
        <td className="history-score-total">{total}</td></tr>;
    })}</tbody>
  </table>;
}

function DraftEntry({ entry, before, complete, player, onInspect }: {
  entry: Entry; before?: HistoricalTable; complete: boolean; player: PlayerLabel; onInspect: (index: number) => void;
}) {
  const table = entry.event.table;
  const move = table.lastMove;
  if (!move) return null;
  const actor = player(move.seat);
  const source = move.source === 0 ? "Center" : `Factory ${move.source}`;
  const target = move.row === 0 ? "Floor" : `Line ${move.row}`;
  const effects = historyDraftEffects(table, before);
  const ordinal = `${complete ? "Move" : "Recorded move"} ${entry.moveNumber}`;
  return <button type="button" className="history-draft-entry" data-history-index={entry.index}
    onClick={() => onInspect(entry.index)}
    aria-label={`${ordinal}. View the board after ${actor === "You" ? "you" : actor} took ${move.count} ${move.color} from ${source} to ${target}. ${effects.join(". ")}`}>
    <Ceramic color={move.color} />
    <span className="history-draft-copy">
      <span><strong>{actor}</strong> took {move.count} {move.color}</span>
      <span className="history-draft-route">{source} → {target}</span>
      {effects.length > 0 && <small>{effects.join(" · ")}</small>}
    </span>
    <span className="history-draft-link" aria-hidden="true"><span>{!complete && <small>Recorded </small>}#{entry.moveNumber}</span><TableIcon name="forward" /></span>
  </button>;
}

export function MoveHistorySheet({ history, names, yourSeat, returnLabel, onInspect, onClose }: {
  history: MatchHistory; names: [string, string]; yourSeat: number; returnLabel: string; onInspect: (index: number) => void; onClose: () => void;
}) {
  const player: PlayerLabel = seat => seat === yourSeat ? "You" : names[seat - 1];
  const { events, complete } = history;
  const latest = events.at(-1)?.table;
  const rounds: Round[] = [];
  let moveNumber = 0;
  events.forEach((event, index) => {
    let round = rounds.at(-1);
    if (!round || round.number !== event.table.round) {
      round = { number: event.table.round, entries: [], scored: false };
      rounds.push(round);
    }
    if (event.kind === "draft") moveNumber++;
    if (event.kind === "scoring") round.scored = true;
    round.entries.push({ event, index, moveNumber });
  });
  return <section className="move-history-sheet" aria-labelledby="move-history-title">
    <header className="move-history-heading">
      <div><h2 id="move-history-title" tabIndex={-1}>Move history</h2>
        <p>Newest first · Select a draft to review.</p></div>
      <button type="button" className="history-close" aria-label="Close move history" onClick={onClose}>
        <svg aria-hidden="true" focusable="false" viewBox="0 0 20 20" width="17" height="17" fill="none" stroke="currentColor" strokeWidth="1.7"><path d="m4 4 12 12M16 4 4 16"/></svg>
      </button>
    </header>
    <div className="move-history-events">
      {!complete && <p className="history-recording-note">Partial history · Numbers count recorded moves.</p>}
      {moveNumber === 0 && <p className="history-recording-note">{latest?.finished ? "No drafts were recorded for this match." : "The next draft will appear here."}</p>}
      {rounds.reverse().map(round => <section key={round.number} aria-label={`Round ${round.number}`}>
        <header className="history-round-heading"><h3>Round {round.number}</h3>
          <span>{round.scored ? "Scored" : round.number !== latest?.round ? "Partially recorded" : latest?.finished ? "Finished" : "In progress"}</span>
        </header>
        <ol className="history-round-entries">
          {round.entries.reverse().map(entry => {
            const { event, index } = entry;
            const { table, kind } = event;
            const before = events[index - 1]?.table;
            if (kind === "baseline") return <li key={index} className="history-round-note">
              {index === 0 ? "Recording began here. Earlier moves are unavailable." : "Recording resumed here after a gap."}
            </li>;
            if (kind === "draft") return <li key={index}><DraftEntry entry={entry} before={before}
              complete={complete} player={player} onInspect={onInspect} /></li>;
            if (kind === "scoring") return <li key={index}><RoundScores table={table} before={before} player={player} /></li>;
            if (kind === "final") return <li key={index} className="history-round-note">
              <strong>With final bonuses</strong> · {table.players.map((board, i) => `${player(i + 1)} ${board.score}`).join(" · ")}
            </li>;
            const factoryCount = table.factories.filter(factory => factory.length > 0).length;
            return <li key={index} className="history-round-note">
              <strong>{kind === "opening" ? "Opening deal" : `${factoryCount} ${factoryCount === 1 ? "factory" : "factories"} refilled`}</strong>
              {" · "}{player(table.turn)} {table.turn === yourSeat ? "start" : "starts"} round {table.round}
            </li>;
          })}
        </ol>
      </section>)}
    </div>
    <footer className="move-history-footer"><button type="button" onClick={onClose}><TableIcon name="back" />{returnLabel}</button></footer>
  </section>;
}
