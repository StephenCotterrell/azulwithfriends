package game

import (
	"slices"
	"testing"
)

func TestBotAccountsForOverflowWhenScoringTheLastDraft(t *testing.T) {
	s := State{Table: Table{Round: 3, Turn: 2, MarkerInCenter: true}, FirstSeat: 1}
	s.Center = []string{"red", "red", "red"}
	p := &s.Players[1]
	p.Score = 10
	p.Wall[0][0], p.Wall[0][1] = true, true // Row 1 is worth 3 before overflow.
	p.Wall[2][3] = true                     // Row 3 is worth 2 and fits the draft.
	want := Move{0, "red", 3}
	if got, ok := s.BotMove(); !ok || got != want {
		t.Fatalf("BotMove = %+v, %v; want %+v", got, ok, want)
	}
}

func TestBotDeniesOpponentAWinningReply(t *testing.T) {
	s := State{Table: Table{Round: 5, Turn: 1, MarkerInCenter: true}, FirstSeat: 1}
	s.Factories[0] = []string{"blue"}
	s.Factories[1] = []string{"red"}
	// Blue scores more immediately for us, but red lets the opponent finish.
	s.Players[0].Wall[1][0] = true
	s.Players[1].Wall[0] = [5]bool{true, true, false, true, true}
	intermediate, ok := s.BotMoveAtLevel(BotIntermediate)
	if !ok || intermediate.Color != "blue" {
		t.Fatalf("intermediate chose %+v, want to draft blue for its own wall", intermediate)
	}
	move, ok := s.BotMove()
	if !ok || move.Color != "red" || move.Row == 0 {
		t.Fatalf("BotMove = %+v, %v; want to draft red", move, ok)
	}
}

func TestBotSearchOnlyUsesThePublicPosition(t *testing.T) {
	s := newWithSeed([32]byte{5})
	other := s
	other.FirstSeat = 3 - s.FirstSeat
	other.ShuffleSeed = [32]byte{7}
	other.Bag = []string{"black"}
	other.Discard = []string{"red", "red"}
	// Give the live center and floor spare storage that a shallow copy could
	// overwrite while exploring a factory draft or a center draft.
	center := []string{"yellow", "untouched", "untouched", "untouched", "untouched"}
	floor := []string{"white", "untouched", "untouched", "untouched", "untouched"}
	s.Center, other.Center = center[:1], center[:1]
	s.Players[s.Turn-1].Floor, other.Players[s.Turn-1].Floor = floor[:1], floor[:1]
	before := encoded(t, s)
	for _, level := range []BotLevel{BotNoob, BotBeginner, BotIntermediate, BotCompetitive, BotTactical, BotAdvanced, BotMaster} {
		move, ok := s.BotMoveAtLevel(level)
		again, okAgain := other.BotMoveAtLevel(level)
		if !ok || !okAgain || move != again || !slices.Contains(s.legalMoves(), move) {
			t.Fatalf("%s: visible position produced %+v, %+v", level, move, again)
		}
		if encoded(t, s) != before || center[1] != "untouched" || floor[1] != "untouched" {
			t.Fatalf("%s: search changed the live state", level)
		}
	}
}

func TestBotPlaysThroughAFinishedGame(t *testing.T) {
	s := newWithSeed([32]byte{1})
	for turn := 0; !s.Finished; turn++ {
		move, ok := s.BotMove()
		if !ok || turn >= 250 {
			t.Fatalf("bot stalled at round %d turn %d", s.Round, turn)
		}
		apply(t, &s, s.Turn, move)
	}
	if _, ok := s.BotMove(); ok {
		t.Error("bot offered a move after final scoring")
	}
	checkTotal(t, s)
}

func TestBotLevelChangesDraftPriorities(t *testing.T) {
	s := State{Table: Table{Round: 2, Turn: 1, MarkerInCenter: true}}
	s.Factories[0] = []string{"blue"}
	s.Factories[1] = []string{"red", "red"}
	s.Players[0].Wall[1][0] = true // Blue on line 1 scores two wall points.
	for _, tc := range []struct {
		level BotLevel
		want  Move
	}{
		{BotNoob, Move{1, "blue", 1}},         // First complete line without waste.
		{BotBeginner, Move{2, "red", 2}},      // More tiles fit this complete line.
		{BotIntermediate, Move{1, "blue", 1}}, // Higher personal score.
	} {
		if got, ok := s.BotMoveAtLevel(tc.level); !ok || got != tc.want {
			t.Errorf("%s chose %+v, want %+v", tc.level, got, tc.want)
		}
	}
}

func TestNoobPrioritizesCompletionOverWaste(t *testing.T) {
	s := State{Table: Table{Turn: 1}}
	s.Factories[0] = []string{"blue", "blue"}
	s.Players[0].Wall[1][1] = true // Blue is unavailable on line 2.
	// Line 1 completes with one overflow tile. Line 3 fits both tiles.
	noob, _ := s.BotMoveAtLevel(BotNoob)
	beginner, _ := s.BotMoveAtLevel(BotBeginner)
	if noob.Row != 1 || beginner.Row != 3 {
		t.Fatalf("noob chose %+v, beginner chose %+v", noob, beginner)
	}
}

func TestStrongerBotsFindAForcedWinThreeDraftsAhead(t *testing.T) {
	s := State{Table: Table{Round: 5, Turn: 1, MarkerInCenter: true}}
	s.Factories[0], s.Factories[1], s.Factories[2] = []string{"blue"}, []string{"yellow"}, []string{"red"}
	ours, theirs := &s.Players[0], &s.Players[1]
	ours.Score, theirs.Score = 5, 11
	ours.Wall[0] = [5]bool{true, true, false, true, true}
	ours.Lines[0] = Line{"red", 1} // This is the last round.
	ours.Lines[2], theirs.Lines[2] = Line{"blue", 2}, Line{"yellow", 2}
	theirs.Wall[1][2], theirs.Wall[2][2] = true, true

	for _, level := range []BotLevel{BotCompetitive, BotTactical, BotAdvanced, BotMaster} {
		first, ok := s.BotMoveAtLevel(level)
		if !ok {
			t.Fatalf("%s missed its turn", level)
		}
		// Competitive takes blue now. Denying red first lets the bot win
		// whatever the opponent does. Verify by playing out the real rules.
		afterFirst := s
		apply(t, &afterFirst, 1, first)
		canForceWin := true
		for _, reply := range afterFirst.legalMoves() {
			afterReply := afterFirst
			apply(t, &afterReply, 2, reply)
			canWin := false
			for _, last := range afterReply.legalMoves() {
				result := afterReply
				apply(t, &result, 1, last)
				if result.Finished && slices.Equal(result.Winners, []int{1}) {
					canWin = true
					break
				}
			}
			if !canWin {
				canForceWin = false
				break
			}
		}
		if canForceWin != (level != BotCompetitive) {
			t.Errorf("%s chose %+v, forced win = %v", level, first, canForceWin)
		}
	}
}
