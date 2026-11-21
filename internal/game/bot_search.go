package game

import (
	"math"
	"slices"
)

// searchDraft starts with the original two-ply decision. Each deeper iteration
// searches its predecessor's best root move first for earlier alpha-beta cuts.
// Count simulated drafts instead of elapsed time so load cannot change a move.
func (s *State) searchDraft(moves []Move, maxDepth, budget int) Move {
	boards := [2]boardProjection{projectBoard(s.Players[0]), projectBoard(s.Players[1])}
	search := draftSearch{seat: s.Turn, remaining: budget}
	best := moves[0]
	for depth := 2; depth <= maxDepth; depth++ {
		search.reachedHorizon = false
		move, _, complete := search.visit(s, boards, moves, depth, math.Inf(-1), math.Inf(1))
		if !complete {
			break
		}
		best = move
		if !search.reachedHorizon || depth == maxDepth {
			break
		}
		i := slices.Index(moves, best)
		copy(moves[1:i+1], moves[:i])
		moves[0] = best
	}
	return best
}

type draftSearch struct {
	seat           int
	remaining      int
	reachedHorizon bool
}

// visit returns a value from the drafting seat's perspective (negamax).
// Only the player who drafts needs a fresh board projection.
func (q *draftSearch) visit(s *State, boards [2]boardProjection, moves []Move, depth int, alpha, beta float64) (Move, float64, bool) {
	best, bestValue := moves[0], math.Inf(-1)
	for _, move := range moves {
		if q.remaining == 0 {
			return Move{}, 0, false
		}
		q.remaining--
		next := s.simulateDraft(move)
		boards[s.Turn-1] = projectBoard(next.Players[s.Turn-1])
		empty := next.offersEmpty()
		var value float64
		if depth == 1 || empty {
			if !empty {
				q.reachedHorizon = true
			}
			value = next.botValue(q.seat, boards[q.seat-1], boards[2-q.seat])
			if s.Turn != q.seat {
				value = -value
			}
		} else {
			_, reply, complete := q.visit(&next, boards, next.legalMoves(), depth-1, -beta, -alpha)
			if !complete {
				return Move{}, 0, false
			}
			value = -reply
		}
		if value > bestValue {
			best, bestValue = move, value
		}
		alpha = max(alpha, value)
		if alpha >= beta {
			break
		}
	}
	return best, bestValue, true
}
