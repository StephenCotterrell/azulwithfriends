import type { MatchHistory } from "./api";
import "./score-history.css";

type Point = {
  kind: "opening" | "baseline" | "scoring" | "final";
  round: number;
  label: string;
  tick: string;
  scores: [number, number];
};

function checkpoints(history: MatchHistory): Point[] {
  const points: Point[] = [];
  for (const { kind, table } of history.events) {
    if (kind !== "opening" && kind !== "baseline" && kind !== "scoring" && kind !== "final") continue;
    const round = kind === "scoring" ? table.lastRound?.round ?? table.round : table.round;
    const [label, tick] = kind === "opening" ? ["Opening", "Start"]
      : kind === "baseline" ? [table.finished ? "Final scores" : `Recorded position in round ${round}`, table.finished ? "Final" : "From"]
      : kind === "scoring" ? [`Round ${round}`, `R${round}`]
      : ["Final, including bonuses", "Final"];
    points.push({ kind, round, label, tick, scores: [table.players[0].score, table.players[1].score] });
  }
  return points;
}

// Connect only consecutive known checkpoints. A missing scoring record cannot establish a bonus leg.
function follows(previous: Point, next: Point) {
  if (next.kind === "final") return previous.kind === "scoring" && previous.round === next.round;
  if (next.kind !== "scoring") return false;
  return next.round === (previous.kind === "scoring" ? previous.round + 1 : previous.round);
}

const seats = [0, 1] as const;

export function ScoreHistory({ history, names, yourSeat }: {
  history: MatchHistory; names: [string, string]; yourSeat: number;
}) {
  const points = checkpoints(history);
  if (points.length < 2) return <p className="score-history-note">Round-by-round scores weren’t recorded for this match.</p>;

  const labels = names.map((name, i) => yourSeat === i + 1 ? "You" : name);
  const first = points[0], last = points[points.length - 1], previous = points[points.length - 2];
  const bonusLeg = last.kind === "final" && follows(previous, last);
  const gap = points.some((point, i) => i > 0 && !follows(points[i-1], point));
  const highest = points.reduce((high, point) => Math.max(high, ...point.scores), 0);
  const step = highest > 15 ? Math.ceil(highest / 15) * 5 : Math.max(1, Math.ceil(highest / 3));
  const x = (index: number) => 100 * index / (points.length - 1);
  const y = (score: number) => 100 - 100 * score / (3 * step);
  // Leave space for the two final numbers even when both players finish one point apart.
  const ends = last.scores.map(y);
  const tied = last.scores[0] === last.scores[1];
  if (!tied && Math.abs(ends[0] - ends[1]) < 16) {
    const higher = ends[0] < ends[1] ? 0 : 1;
    ends[higher] = Math.max(0, Math.min(84, (ends[0] + ends[1])/2 - 8));
    ends[1-higher] = ends[higher] + 16;
  }
  const labelInterval = Math.ceil((points.length - 1) / 6);
  const showTick = (i: number) => i === points.length - 1
    || (i % labelInterval === 0 && points.length - 1 - i >= labelInterval);
  return <section className="score-history" aria-label="Score history">
    {(!history.complete || first.kind !== "opening") && <p className="score-history-note">
      {first.kind === "opening" ? "Part of this match was played while history was unavailable."
        : `Recording began in round ${first.round}. Earlier scores are unavailable.`}
    </p>}
    {gap && <p className="score-history-note">Gaps in the lines mark missing checkpoints.</p>}
    <div className="score-history-card">
      <header className="score-history-heading"><h3>Score by round</h3><span>Points</span></header>
      <ul className="score-history-key" aria-label="Players">{seats.map(i => <li key={i}>
        <i className={`score-history-sample series-${i}`} aria-hidden="true" />{labels[i]}</li>)}</ul>
      <div className="score-history-figure" role="img" aria-label={`Score chart. Last recorded scores: ${labels[0]} ${last.scores[0]}, ${labels[1]} ${last.scores[1]}. Expand Checkpoint scores for the full data.`}>
        <div className="score-history-plot">
          {bonusLeg && <div className="score-history-bonus-band" style={{ left: x(points.length - 2)+"%" }} />}
          {[0, step, step*2, step*3].map(value => <div className="score-history-rule" key={value} style={{ top:y(value)+"%" }}><span>{value}</span></div>)}
          <svg className="score-history-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            {seats.map(i => <path key={i} className={`series-${i}`} vectorEffect="non-scaling-stroke" d={points.map((p,j) =>
              `${j && follows(points[j-1],p) ? "L" : "M"}${x(j)},${y(p.scores[i])}`).join(" ")} />)}
          </svg>
          {points.map((p,j) => <div key={j}>
            {seats.map(i => <i key={i} className={`score-history-point series-${i}`} style={{ left:x(j)+"%", top:y(p.scores[i])+"%" }}/>)}
            {showTick(j) && <span className="score-history-tick" style={{ left:x(j)+"%" }}>{p.tick}</span>}
          </div>)}
          {tied ? <b className="score-history-end" style={{ top:ends[0]+"%" }}>{last.scores[0]}<small>both</small></b>
            : seats.map(i => <b key={i} className={`score-history-end series-${i}`} style={{ top:ends[i]+"%" }}>{last.scores[i]}</b>)}
        </div>
      </div>
      {bonusLeg && <div className="score-history-bonuses"><b>Final bonuses</b>{seats.map(i => <span key={i}>
        {labels[i]} +{last.scores[i] - previous.scores[i]}</span>)}</div>}
    </div>
    <details className="score-history-data"><summary>Checkpoint scores</summary>
      <table><caption className="sr-only">Cumulative points at each recorded checkpoint</caption>
        <thead><tr><th scope="col">Checkpoint</th>{seats.map(i => <th scope="col" key={i}>{labels[i]}</th>)}</tr></thead>
        <tbody>{points.map((p,i) => <tr key={i}><th scope="row">{p.label}</th>{p.scores.map((s,j) => <td key={j}>{s}</td>)}</tr>)}</tbody>
      </table>
    </details>
  </section>;
}
