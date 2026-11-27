import type { HistoricalTable } from "./api";

// Both snapshots are from the same draft, before any end-of-round scoring.
export function historyDraftEffects(table: HistoricalTable, before?: HistoricalTable): string[] {
  const move = table.lastMove;
  if (!move || !before) return [];
  const prior = before.players[move.seat - 1];
  const board = table.players[move.seat - 1];
  const addedToFloor = board.floor.slice(prior.floor.length);
  const effects: string[] = [];
  if (move.source === 0 && before.markerInCenter) {
    effects.push(addedToFloor.includes("first") ? "First-player marker to floor" : "First-player marker (no floor penalty)");
  }
  const placed = move.row ? board.lines[move.row - 1].count - prior.lines[move.row - 1].count : 0;
  const onFloor = addedToFloor.filter(tile => tile === move.color).length;
  const discarded = move.count - placed - onFloor;
  if (move.row && onFloor > 0) effects.push(`${onFloor} to floor`);
  if (discarded > 0) effects.push(`${discarded} discarded (floor full)`);
  return effects;
}
