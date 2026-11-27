package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRecordedMatch(t *testing.T) {
	var seed [32]byte
	copy(seed[:], "azul-with-friends-history-2026")
	recorded, ordinary := newWithSeed(seed), newWithSeed(seed)
	var drafts, refills int
	for !ordinary.Finished {
		level := BotIntermediate
		if ordinary.Turn == 2 {
			level = BotBeginner
		}
		move, ok := ordinary.BotMoveAtLevel(level)
		if !ok || drafts > 63 {
			t.Fatalf("fixture diverged after %d drafts", drafts)
		}
		before := ordinary.PublicTable()
		apply(t, &ordinary, before.Turn, move)
		events, err := recorded.ApplyRecorded(before.Turn, move)
		if err != nil || !reflect.DeepEqual(recorded, ordinary) {
			t.Fatalf("recorded play diverged on move %d: %v", drafts+1, err)
		}
		drafts++
		if len(events) == 0 || events[0].Kind != "draft" || !reflect.DeepEqual(events[len(events)-1].Table, ordinary.PublicTable()) {
			t.Fatalf("last event must match the live table on move %d", drafts)
		}
		draft := events[0].Table
		if draft.Round != before.Round || draft.LastMove == nil || draft.LastMove.Move != move || draft.LastMove.Seat != before.Turn {
			t.Fatalf("incorrect draft identity on move %d", drafts)
		}
		for i := range draft.Players {
			if draft.Players[i].Score != before.Players[i].Score || draft.Players[i].Wall != before.Players[i].Wall {
				t.Fatalf("draft includes premature scoring on move %d", drafts)
			}
		}
		if ordinary.Round == before.Round && !ordinary.Finished {
			if len(events) != 1 {
				t.Fatalf("ordinary draft emitted %d events", len(events))
			}
			continue
		}
		if len(events) != 3 || events[1].Kind != "scoring" {
			t.Fatalf("missing round scoring on move %d", drafts)
		}
		scored := events[1].Table
		if scored.Turn != 0 || scored.Finished || len(scored.Winners) > 0 || scored.Round != before.Round ||
			scored.LastRound == nil || *scored.LastRound != *ordinary.LastRound || len(scored.Center) > 0 {
			t.Fatalf("invalid scoring phase on move %d", drafts)
		}
		for _, factory := range scored.Factories {
			if len(factory) > 0 {
				t.Fatalf("round %d was dealt before scoring", scored.Round)
			}
		}
		for i, p := range scored.Players {
			points := scored.LastRound.Players[i]
			if p.Score != before.Players[i].Score+points.Wall-points.Penalty || p.Bonus != (Bonus{}) || len(p.Floor) > 0 {
				t.Fatalf("wrong pre-bonus score or floor for seat %d in round %d", i+1, scored.Round)
			}
		}
		if ordinary.Finished {
			if events[2].Kind != "final" {
				t.Fatalf("expected final event, got %s", events[2].Kind)
			}
		} else {
			if events[2].Kind != "refill" || !events[2].Table.MarkerInCenter || len(events[2].Table.Factories[0]) != 4 {
				t.Fatalf("invalid refill on move %d", drafts)
			}
			refills++
		}
	}
	if drafts != 63 || refills != 4 || ordinary.Players[0].Score != 30 || ordinary.Players[1].Score != 56 {
		t.Fatal("recorded match must retain the original fixture outcome")
	}
}

func TestHistoryIsPublicAndIndependent(t *testing.T) {
	s := State{Table: Table{Round: 2, Turn: 1, Center: []string{"white"}, LastRound: &RoundResult{Round: 1}}}
	s.Factories[0] = []string{"red", "red", "blue"}
	s.Factories[1] = []string{"black"}
	events, err := s.ApplyRecorded(1, Move{1, "red", 1})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one accepted draft, got %v, %v", events, err)
	}
	board := &events[0].Table
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encoded(t, board)), &fields); err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"bag", "discard", "shuffleSeed", "firstSeat", "legalMoves", "anticipatedScores"} {
		if _, ok := fields[excluded]; ok {
			t.Errorf("history exposed %s", excluded)
		}
	}
	before := encoded(t, s)
	board.Center[0] = "changed"
	board.Factories[1][0] = "changed"
	board.Players[0].Floor[0] = "changed"
	board.LastMove.Count = 0
	board.LastRound.Round = 0
	if encoded(t, s) != before {
		t.Fatal("editing the snapshot changed the game")
	}
	if rejected, err := s.ApplyRecorded(1, Move{0, "white", 1}); err == nil || len(rejected) > 0 || encoded(t, s) != before {
		t.Fatal("an out-of-turn draft changed the game or emitted history")
	}
}
