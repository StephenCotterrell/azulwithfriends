package game

import (
	"cmp"
	"math"
	"slices"
)

type BotLevel string

const (
	BotNoob         BotLevel = "noob"
	BotBeginner     BotLevel = "beginner"
	BotIntermediate BotLevel = "intermediate"
	BotCompetitive  BotLevel = "competitive"
	BotTactical     BotLevel = "tactical"
	BotAdvanced     BotLevel = "advanced"
	BotMaster       BotLevel = "master"
)

func (level BotLevel) Valid() bool {
	switch level {
	case BotNoob, BotBeginner, BotIntermediate, BotCompetitive, BotTactical, BotAdvanced, BotMaster:
		return true
	}
	return false
}

// BotMove preserves the original bot's two-draft search and tie breaking.
func (s State) BotMove() (Move, bool) {
	return s.BotMoveAtLevel(BotCompetitive)
}

func (s State) BotMoveAtLevel(level BotLevel) (Move, bool) {
	position := State{Table: s.Table}
	moves := position.legalMoves()
	if len(moves) == 0 {
		return Move{}, false
	}
	switch level {
	case BotNoob, BotBeginner, BotIntermediate:
		return position.simpleDraft(level, moves), true
	case BotCompetitive:
		return position.searchDraft(moves, 2, math.MaxInt), true
	case BotTactical:
		return position.searchDraft(moves, 3, 24_000), true
	case BotAdvanced:
		return position.searchDraft(moves, 5, 60_000), true
	case BotMaster:
		// A round starts with at most 20 colored tiles, one per draft at minimum.
		return position.searchDraft(moves, 20, 150_000), true
	default:
		return Move{}, false
	}
}

func (s *State) simpleDraft(level BotLevel, moves []Move) Move {
	best, bestValue, leastSpill := moves[0], math.Inf(-1), 0
	for _, move := range moves {
		next := s.simulateDraft(move)
		player := next.Players[s.Turn-1]
		var value float64
		var spill int
		switch level {
		case BotNoob, BotBeginner:
			placed := 0
			if move.Row > 0 {
				placed = player.Lines[move.Row-1].Count - s.Players[s.Turn-1].Lines[move.Row-1].Count
			}
			spill = next.LastMove.Count - placed
			if level == BotBeginner {
				value = float64(placed)
			}
			// A completion wins a tie on quantity for Beginner and is
			// Noob's primary goal. Both then minimize wasted colored tiles.
			if move.Row > 0 && player.Lines[move.Row-1].Count == move.Row {
				value += 0.5
			}
		case BotIntermediate:
			board := projectBoard(player)
			value = float64(board.score) + board.progress
		}
		if value > bestValue || (value == bestValue && spill < leastSpill) {
			best, bestValue, leastSpill = move, value, spill
		}
	}
	return best
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
