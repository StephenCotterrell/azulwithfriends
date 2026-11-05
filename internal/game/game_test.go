package game

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestTakeFromFactoryThenCenter(t *testing.T) {
	s := State{Table: Table{Round: 1, Turn: 1, MarkerInCenter: true}, FirstSeat: 1}
	s.Factories[0] = []string{"red", "blue", "red", "blue"}
	s.Factories[1] = []string{"white"} // Keep this round in progress.
	s.Center = []string{"yellow"}

	apply(t, &s, 1, Move{1, "red", 1})
	if len(s.Factories[0]) != 0 || !slices.Equal(s.Center, []string{"yellow", "blue", "blue"}) {
		t.Fatalf("factory leftovers: %+v", s.Table)
	}
	if *s.LastMove != (Draft{Move: Move{1, "red", 1}, Seat: 1, Count: 2}) {
		t.Fatalf("incorrect draft summary: %+v", s.LastMove)
	}
	if s.Players[0].Lines[0] != (Line{"red", 1}) || !slices.Equal(s.Players[0].Floor, []string{"red"}) {
		t.Fatalf("excess tile not sent to floor: %+v", s.Players[0])
	}
	apply(t, &s, 2, Move{0, "blue", 0})
	if !slices.Equal(s.Center, []string{"yellow"}) || !slices.Equal(s.Players[1].Floor, []string{firstMarker, "blue", "blue"}) {
		t.Fatalf("center draft: %+v", s.Table)
	}
	apply(t, &s, 1, Move{0, "yellow", 2})
	if s.MarkerInCenter || s.FirstSeat != 2 || s.Players[0].Lines[1] != (Line{"yellow", 1}) || len(s.Players[0].Floor) != 1 {
		t.Fatalf("wrong marker or destination: %+v", s)
	}
}

func TestInvalidMoveDoesNotChangeState(t *testing.T) {
	s := State{Table: Table{Round: 1, Turn: 1, MarkerInCenter: true}}
	s.Factories[0] = []string{"red", "yellow"}
	p := &s.Players[0]
	p.Lines[0] = Line{"red", 1}
	p.Lines[1] = Line{"yellow", 1}
	p.Wall[2][4] = true // Red on row 3.
	before := encoded(t, s)
	for _, tc := range []struct {
		seat int
		move Move
	}{
		{2, Move{1, "red", 4}}, {0, Move{1, "red", 4}},
		{1, Move{-1, "red", 4}}, {1, Move{6, "red", 4}}, {1, Move{0, "red", 4}},
		{1, Move{1, "green", 4}}, {1, Move{1, "blue", 4}},
		{1, Move{1, "red", -1}}, {1, Move{1, "red", 6}},
		{1, Move{1, "red", 1}}, {1, Move{1, "red", 2}}, {1, Move{1, "red", 3}},
	} {
		if err := s.Apply(tc.seat, tc.move); err == nil || encoded(t, s) != before {
			t.Errorf("accepted or mutated for seat %d: %+v", tc.seat, tc.move)
		}
		if tc.seat == s.Turn && slices.Contains(s.View().LegalMoves, tc.move) {
			t.Errorf("advertised illegal move %+v", tc.move)
		}
	}
	apply(t, &s, 1, Move{1, "yellow", 2})
	if p.Lines[1] != (Line{"yellow", 2}) {
		t.Errorf("existing yellow line was not completed: %+v", p.Lines)
	}
}

func TestWallOrderAndFloorPenalties(t *testing.T) {
	s := State{Table: Table{Round: 1, Turn: 1}, FirstSeat: 2, Bag: []string{"black"}}
	p := &s.Players[0]
	p.Score = 10
	p.Wall[0][1] = true
	p.Lines[0] = Line{"blue", 1}
	p.Lines[1] = Line{"white", 2}
	p.Lines[2] = Line{"red", 1}
	p.Floor = []string{"black", firstMarker, "blue"}
	// Row 1 scores two horizontally. Row 2 then scores two vertically.
	s.finishRound()
	if s.LastRound.Players[0] != (RoundPoints{Wall: 4, Penalty: 4}) || p.Score != 10 || !p.Wall[0][0] || !p.Wall[1][0] {
		t.Fatalf("wrong round result: %+v, board %+v", s.LastRound, p)
	}
	if p.Lines[0] != (Line{}) || p.Lines[1] != (Line{}) || p.Lines[2] != (Line{"red", 1}) || len(p.Floor) != 0 {
		t.Fatalf("wrong round cleanup: %+v", p)
	}
	if s.Round != 2 || s.Turn != 2 || !s.MarkerInCenter || len(s.Factories[0]) != 4 {
		t.Fatalf("expected partial deal after refill, with seat 2 starting: %+v", s.Table)
	}
	for _, factory := range s.Factories[1:] {
		if len(factory) != 0 {
			t.Fatalf("dealt tiles with an empty bag and lid: %+v", s.Factories)
		}
	}
	if slices.Contains(s.Factories[0], firstMarker) {
		t.Error("starting marker was shuffled into the bag")
	}
}

func TestFullFloorOverflowAndStartPrivilege(t *testing.T) {
	s := State{Table: Table{Round: 1, Turn: 2, MarkerInCenter: true}, FirstSeat: 1}
	s.Factories[0] = []string{"blue"}
	s.Center = []string{"red", "red"}
	s.Players[1].Floor = []string{"white", "white", "white", "white", "white", "white", "white"}
	apply(t, &s, 2, Move{0, "red", 0})
	if len(s.Players[1].Floor) != 7 || !slices.Equal(s.Discard, []string{"red", "red"}) || s.FirstSeat != 2 {
		t.Fatalf("full floor overflow/marker: %+v", s)
	}
	apply(t, &s, 1, Move{1, "blue", 0})
	if s.Round != 2 || s.Turn != 2 || s.Players[1].Score != 0 || s.LastRound.Players[1].Penalty != 0 {
		t.Fatalf("marker holder should start despite full floor and score clamp: %+v", s.Table)
	}
	// With seven occupied positions the printed cost is 14 points.
	p := Player{Score: 15, Floor: []string{firstMarker, "red", "red", "red", "red", "red", "red"}}
	if got := s.tileWall(&p); got.Penalty != 14 || p.Score != 1 {
		t.Fatalf("full floor deduction = %+v, remaining score %d", got, p.Score)
	}
}

func TestNoCenterDraftRetainsStartingSeat(t *testing.T) {
	s := State{Table: Table{Round: 1, Turn: 1, MarkerInCenter: true}, FirstSeat: 2}
	s.Factories[0] = []string{"white"}
	apply(t, &s, 1, Move{1, "white", 0})
	if s.Round != 2 || s.Turn != 2 || !s.MarkerInCenter {
		t.Fatalf("wrong start when every factory is monochrome: %+v", s.Table)
	}
}

func TestFinalScoringAndBonuses(t *testing.T) {
	s := State{Table: Table{Round: 7, Turn: 2}, FirstSeat: 1}
	p := &s.Players[0]
	p.Score = 5
	for i := range 4 {
		p.Wall[4][i] = true // A row, column, and set of blues await a final blue.
		p.Wall[i][4] = true
		p.Wall[i][i] = true
	}
	p.Lines[4] = Line{"blue", 5}
	s.Factories[0] = []string{"red"}
	apply(t, &s, 2, Move{1, "red", 1})
	if !s.Finished || s.Round != 7 || s.Turn != 0 || !slices.Equal(s.Winners, []int{1}) {
		t.Fatalf("wrong end of game: %+v", s.Table)
	}
	if p.Bonus != (Bonus{Rows: 2, Columns: 7, Colors: 10}) || s.LastRound.Players[0].Wall != 10 || p.Score != 34 {
		t.Fatalf("row, column, color, or crossing points incorrect: %+v, %+v", p, s.LastRound)
	}
	if s.Players[1].Score != 1 || !s.Players[1].Wall[0][2] {
		t.Fatal("the second player's final placement was skipped")
	}
	before := encoded(t, s)
	if len(s.View().LegalMoves) != 0 || s.Apply(1, Move{1, "red", 0}) == nil || encoded(t, s) != before {
		t.Fatal("finished games must stay at their final result")
	}
}

func TestTiebreak(t *testing.T) {
	tests := []struct {
		players [2]Player
		want    []int
	}{
		{[2]Player{{Score: 45, Bonus: Bonus{Rows: 4}}, {Score: 46, Bonus: Bonus{Rows: 2}}}, []int{2}},
		{[2]Player{{Score: 45, Bonus: Bonus{Rows: 2}}, {Score: 45, Bonus: Bonus{Rows: 4}}}, []int{2}},
		{[2]Player{{Score: 45, Bonus: Bonus{Rows: 2}}, {Score: 45, Bonus: Bonus{Rows: 2}}}, []int{1, 2}},
	}
	for _, tc := range tests {
		if got := winners(tc.players); !slices.Equal(got, tc.want) {
			t.Errorf("winners = %v, want %v", got, tc.want)
		}
	}
}

func TestPublicSnapshot(t *testing.T) {
	s := newWithSeed([32]byte{7})
	v := s.View()
	if s.Round != 1 || (s.Turn != 1 && s.Turn != 2) || len(s.Bag) != 80 || !s.MarkerInCenter {
		t.Fatalf("unexpected setup: %+v", s)
	}
	for _, factory := range v.Factories {
		if len(factory) != 4 {
			t.Fatalf("bad initial factory: %v", factory)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encoded(t, v)), &fields); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"bag", "discard", "shuffleSeed", "firstSeat"} {
		if _, ok := fields[private]; ok {
			t.Errorf("view leaks %s", private)
		}
	}
	if string(fields["winners"]) != "[]" || string(fields["center"]) != "[]" || v.Players[0].Floor == nil {
		t.Fatal("empty public arrays should encode as []")
	}
	before := encoded(t, v)
	apply(t, &s, s.Turn, v.LegalMoves[0])
	if encoded(t, v) != before {
		t.Fatal("a previously returned view changed after a move")
	}
}

// Two players complete the top row when possible. This exercises multiple
// distributions, including bag refills, while making the whole game repeatable.
func TestCompleteGamesAcrossSaveAndRestore(t *testing.T) {
	// Seed 56 ends in a shared victory with this drafting policy.
	for seed := range 57 {
		s := newWithSeed([32]byte{byte(seed)})
		var restored State
		for turn := 0; !s.Finished; turn++ {
			if turn >= 250 {
				t.Fatalf("seed %d never finished", seed)
			}
			checkTotal(t, s)
			if err := json.Unmarshal([]byte(encoded(t, s)), &restored); err != nil {
				t.Fatal(err)
			}
			moves := s.View().LegalMoves
			if len(moves) == 0 {
				t.Fatal("active position has no draft")
			}
			move := moves[0]
			for _, option := range moves {
				if option.Row > 0 && (move.Row == 0 || option.Row < move.Row) {
					move = option
				}
			}
			apply(t, &s, s.Turn, move)
			apply(t, &restored, restored.Turn, move)
			if !reflect.DeepEqual(s, restored) {
				t.Fatalf("restored game diverged on seed %d turn %d", seed, turn)
			}
		}
		checkTotal(t, s)
		if len(s.Winners) == 0 || s.Round < 5 {
			t.Fatalf("incomplete final result: %+v", s.Table)
		}
		if seed == 56 && (!slices.Equal(s.Winners, []int{1, 2}) || s.Players[0].Score != s.Players[1].Score) {
			t.Fatalf("expected shared victory from the opening deal: %+v", s.Table)
		}
	}
}

func checkTotal(t *testing.T, s State) {
	t.Helper()
	count := map[string]int{}
	for _, tiles := range [][]string{s.Bag, s.Discard, s.Center} {
		for _, c := range tiles {
			count[c]++
		}
	}
	for _, factory := range s.Factories {
		for _, c := range factory {
			count[c]++
		}
	}
	for _, p := range s.Players {
		for _, c := range p.Floor {
			if c != firstMarker {
				count[c]++
			}
		}
		for r, line := range p.Lines {
			if line.Count > 0 {
				count[line.Color] += line.Count
			}
			for c, filled := range p.Wall[r] {
				if filled {
					count[colors[(c-r+5)%5]]++
				}
			}
		}
	}
	for _, color := range colors {
		if count[color] != 20 {
			t.Fatalf("expected 20 of each color: %v", count)
		}
	}
	if len(count) != 5 {
		t.Fatalf("unexpected colored tiles: %v", count)
	}
}

func apply(t *testing.T, s *State, seat int, move Move) {
	t.Helper()
	if err := s.Apply(seat, move); err != nil {
		t.Fatalf("seat %d, move %+v: %v", seat, move, err)
	}
}

func encoded(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
