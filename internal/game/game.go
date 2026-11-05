// Package game implements a two-player game on Azul's standard colored wall.
package game

import (
	"crypto/rand"
	"errors"
	mathrand "math/rand/v2"
	"slices"
)

const firstMarker = "first"

var colors = [...]string{"blue", "yellow", "red", "black", "white"}

// Source zero selects the center, and sources 1–5 select the factories.
// Row zero selects the floor, and rows 1–5 select the pattern lines.
type Move struct {
	Source int    `json:"source"`
	Color  string `json:"color"`
	Row    int    `json:"row"`
}

type Draft struct {
	Move
	Seat  int `json:"seat"`
	Count int `json:"count"`
}

type Line struct {
	Color string `json:"color"`
	Count int    `json:"count"`
}

type Player struct {
	Score int        `json:"score"`
	Lines [5]Line    `json:"lines"`
	Wall  [5][5]bool `json:"wall"`
	Floor []string   `json:"floor"`
	Bonus Bonus      `json:"bonus"`
}

// Bonuses are points already awarded at the end of the game.
type Bonus struct {
	Rows    int `json:"rows"`
	Columns int `json:"columns"`
	Colors  int `json:"colors"`
}

type RoundPoints struct {
	Wall    int `json:"wall"`
	Penalty int `json:"penalty"` // Actual deduction after applying the zero-point minimum.
}

type RoundResult struct {
	Round   int            `json:"round"`
	Players [2]RoundPoints `json:"players"`
}

type Table struct {
	Round          int          `json:"round"`
	Turn           int          `json:"turn"`
	Finished       bool         `json:"finished"`
	Factories      [5][]string  `json:"factories"`
	Center         []string     `json:"center"`
	MarkerInCenter bool         `json:"markerInCenter"`
	Players        [2]Player    `json:"players"`
	Winners        []int        `json:"winners"`
	LastMove       *Draft       `json:"lastMove,omitempty"`
	LastRound      *RoundResult `json:"lastRound,omitempty"`
}

// State is the server's durable JSON snapshot. View is the browser projection.
type State struct {
	Table
	Bag         []string `json:"bag"`
	Discard     []string `json:"discard"`
	ShuffleSeed [32]byte `json:"shuffleSeed"`
	FirstSeat   int      `json:"firstSeat"` // Starts the next round, even if nobody ever drafted from the center.
}

type View struct {
	Table
	LegalMoves []Move `json:"legalMoves"`
}

func New() State {
	var seed [32]byte
	rand.Read(seed[:])
	return newWithSeed(seed)
}

func newWithSeed(seed [32]byte) State {
	s := State{ShuffleSeed: seed}
	for _, color := range colors {
		for range 20 {
			s.Bag = append(s.Bag, color)
		}
	}
	shuffle := s.nextRandom()
	s.FirstSeat = shuffle.IntN(2) + 1
	shuffle.Shuffle(len(s.Bag), func(i, j int) { s.Bag[i], s.Bag[j] = s.Bag[j], s.Bag[i] })
	s.startRound()
	return s
}

// Advance a secret seed once per shuffle so a restored position plays out
// identically. The seed and bag order never enter the public projection.
func (s *State) nextRandom() *mathrand.Rand {
	stream := mathrand.NewChaCha8(s.ShuffleSeed)
	stream.Read(s.ShuffleSeed[:])
	return mathrand.New(stream)
}

func (s *State) startRound() {
	s.Round++
	s.Turn = s.FirstSeat
	s.MarkerInCenter = true
	for i := range s.Factories {
		for len(s.Factories[i]) < 4 {
			if len(s.Bag) == 0 {
				if len(s.Discard) == 0 {
					return // Remaining factories can be partly filled or empty.
				}
				s.Bag, s.Discard = s.Discard, nil
				s.nextRandom().Shuffle(len(s.Bag), func(i, j int) { s.Bag[i], s.Bag[j] = s.Bag[j], s.Bag[i] })
			}
			last := len(s.Bag) - 1
			s.Factories[i] = append(s.Factories[i], s.Bag[last])
			s.Bag = s.Bag[:last]
		}
	}
}

func (s State) View() View {
	t := s.Table
	// Keep the view independent of later mutations and encode empty offers as [].
	t.Center = append([]string{}, t.Center...)
	t.Winners = append([]int{}, t.Winners...)
	for i, factory := range t.Factories {
		t.Factories[i] = append([]string{}, factory...)
	}
	for i, p := range t.Players {
		t.Players[i].Floor = append([]string{}, p.Floor...)
	}
	if t.LastRound != nil {
		result := *t.LastRound
		t.LastRound = &result
	}
	if t.LastMove != nil {
		last := *t.LastMove
		t.LastMove = &last
	}
	return View{Table: t, LegalMoves: s.legalMoves()}
}

func (s *State) source(n int) []string {
	if n == 0 {
		return s.Center
	}
	return s.Factories[n-1]
}

func (s *State) legalMoves() []Move {
	moves := []Move{}
	if s.Finished {
		return moves
	}
	p := &s.Players[s.Turn-1]
	for source := 0; source <= len(s.Factories); source++ {
		tiles := s.source(source)
		for index, color := range colors {
			if !slices.Contains(tiles, color) {
				continue
			}
			for row := 0; row <= len(p.Lines); row++ {
				if p.canPlace(index, row) {
					moves = append(moves, Move{Source: source, Color: color, Row: row})
				}
			}
		}
	}
	return moves
}

func (p *Player) canPlace(color, row int) bool {
	if row == 0 {
		return true
	}
	line := p.Lines[row-1]
	return line.Count < row && (line.Count == 0 || line.Color == colors[color]) && !p.Wall[row-1][wallColumn(row-1, color)]
}

func wallColumn(row, color int) int { return (row + color) % len(colors) }

// Apply validates the entire draft before changing any part of the state.
func (s *State) Apply(seat int, m Move) error {
	if s.Finished {
		return errors.New("the game has finished")
	}
	if seat < 1 || seat > 2 || s.Turn != seat {
		return errors.New("wait for your turn")
	}
	p := &s.Players[seat-1]
	if m.Source < 0 || m.Source > len(s.Factories) || m.Row < 0 || m.Row > len(p.Lines) {
		return errors.New("choose a valid source and destination")
	}
	index := slices.Index(colors[:], m.Color)
	if index < 0 || !slices.Contains(s.source(m.Source), m.Color) {
		return errors.New("that source has no tiles of this color")
	}
	if !p.canPlace(index, m.Row) {
		return errors.New("these tiles cannot go on that pattern line")
	}

	s.applyDraft(m)
	if s.offersEmpty() {
		s.finishRound()
	}
	return nil
}

// applyDraft plays a known legal move. The caller controls when to score and
// refill so a simulated draft can stop at the end of the visible offers.
func (s *State) applyDraft(m Move) {
	seat := s.Turn
	p := &s.Players[seat-1]
	picked := s.take(m.Source, m.Color)
	s.LastMove = &Draft{Move: m, Seat: seat, Count: picked}
	if m.Source == 0 && s.MarkerInCenter {
		s.MarkerInCenter = false
		s.FirstSeat = seat
		if len(p.Floor) < len(floorCosts) {
			p.Floor = append(p.Floor, firstMarker)
		}
	}
	if m.Row != 0 {
		line := &p.Lines[m.Row-1]
		count := min(picked, m.Row-line.Count)
		line.Color = m.Color
		line.Count += count
		picked -= count
	}
	for range picked {
		if len(p.Floor) < len(floorCosts) {
			p.Floor = append(p.Floor, m.Color)
		} else {
			s.Discard = append(s.Discard, m.Color)
		}
	}
	s.Turn = 3 - seat
}

func (s *State) take(source int, color string) int {
	tiles := s.source(source)
	remaining := make([]string, 0, len(tiles))
	for _, tile := range tiles {
		if tile != color {
			remaining = append(remaining, tile)
		}
	}
	if source == 0 {
		s.Center = remaining
	} else {
		s.Factories[source-1] = nil
		s.Center = append(s.Center, remaining...)
	}
	return len(tiles) - len(remaining)
}

func (s *State) offersEmpty() bool {
	for _, factory := range s.Factories {
		if len(factory) > 0 {
			return false
		}
	}
	return len(s.Center) == 0
}
