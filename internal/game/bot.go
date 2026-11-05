package game

import (
	"cmp"
	"math"
	"slices"
)

// BotMove chooses a draft for the current seat. It considers every legal
// draft and the opponent's strongest reply within the current round.
func (s State) BotMove() (Move, bool) {
	position := State{Table: s.Table}
	moves := position.legalMoves()
	if len(moves) == 0 {
		return Move{}, false
	}
	best, bestValue := moves[0], math.Inf(-1)
	for _, move := range moves {
		next := position.simulateDraft(move)
		ours := projectBoard(next.Players[s.Turn-1])
		replies := next.legalMoves()
		value := math.Inf(1)
		if len(replies) == 0 {
			value = next.botValue(s.Turn, ours, projectBoard(next.Players[next.Turn-1]))
		}
		for _, reply := range replies {
			after := next.simulateDraft(reply)
			theirs := projectBoard(after.Players[next.Turn-1])
			value = min(value, after.botValue(s.Turn, ours, theirs))
			if value <= bestValue {
				break // This draft cannot beat the best one already found.
			}
		}
		if value > bestValue {
			best, bestValue = move, value
		}
	}
	return best, true
}

func (s State) simulateDraft(m Move) State {
	// take replaces the source. Cap the two slices a draft can append to so
	// siblings and the live table keep independent contents.
	s.Center = slices.Clip(s.Center)
	p := &s.Players[s.Turn-1]
	p.Floor = slices.Clip(p.Floor)
	s.Discard = nil
	s.applyDraft(m)
	return s
}

type boardProjection struct {
	score    int
	rowBonus int
	progress float64
}

func projectBoard(p Player) boardProjection {
	var round State
	round.tileWall(&p)
	bonus := p.wallBonus()
	projected := boardProjection{score: p.Score + bonus.Rows + bonus.Columns + bonus.Colors, rowBonus: bonus.Rows}
	for row, line := range p.Lines {
		if line.Count > 0 {
			column := wallColumn(row, colorIndex(line.Color))
			points := placementScore(&p.Wall, row, column)
			filled := float64(line.Count) / float64(row+1)
			// Give partial lines some credit for their eventual placement.
			projected.progress += 0.5 * filled * float64(points)
		}
	}
	return projected
}

func (s *State) botValue(seat int, ours, theirs boardProjection) float64 {
	diff := ours.score - theirs.score
	if ours.rowBonus > 0 || theirs.rowBonus > 0 {
		// An already completed row guarantees final scoring this round.
		if s.offersEmpty() {
			outcome := cmp.Compare(ours.score, theirs.score)
			if outcome == 0 {
				outcome = cmp.Compare(ours.rowBonus, theirs.rowBonus)
			}
			return float64(1000*outcome + diff) // Prefer a certain win.
		}
		return float64(diff) + 0.1*float64(cmp.Compare(ours.rowBonus, theirs.rowBonus))
	}
	value := float64(diff) + ours.progress - theirs.progress
	if s.FirstSeat != 0 { // A player claimed next round's opening draft during the search.
		if s.FirstSeat == seat {
			return value + 0.5
		}
		return value - 0.5
	}
	return value
}
