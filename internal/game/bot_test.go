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
	move, ok := s.BotMove()
	again, okAgain := other.BotMove()
	if !ok || !okAgain || move != again || !slices.Contains(s.legalMoves(), move) {
		t.Fatalf("visible position produced %+v, %+v", move, again)
	}
	if encoded(t, s) != before || center[1] != "untouched" || floor[1] != "untouched" {
		t.Fatal("search changed the live state")
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
