package game

var floorCosts = [...]int{1, 1, 2, 2, 2, 3, 3}

func (s *State) finishRound() {
	result := RoundResult{Round: s.Round}
	var bonuses [2]Bonus
	for i := range s.Players {
		p := &s.Players[i]
		result.Players[i] = s.tileWall(p)
		bonuses[i] = p.wallBonus()
	}
	s.LastRound = &result
	if bonuses[0].Rows == 0 && bonuses[1].Rows == 0 {
		s.startRound()
		return
	}
	for i, bonus := range bonuses {
		p := &s.Players[i]
		p.Bonus = bonus
		p.Score += bonus.Rows + bonus.Columns + bonus.Colors
	}
	s.Turn = 0
	s.Finished = true
	s.MarkerInCenter = false
	s.Winners = winners(s.Players)
}

func (s *State) tileWall(p *Player) RoundPoints {
	var points RoundPoints
	for row, line := range p.Lines {
		if line.Count != row+1 {
			continue
		}
		column := wallColumn(row, colorIndex(line.Color))
		p.Wall[row][column] = true
		points.Wall += placementScore(&p.Wall, row, column)
		for range line.Count - 1 {
			s.Discard = append(s.Discard, line.Color)
		}
		p.Lines[row] = Line{}
	}
	p.Score += points.Wall
	for i, tile := range p.Floor {
		points.Penalty += floorCosts[i]
		if tile != firstMarker {
			s.Discard = append(s.Discard, tile)
		}
	}
	points.Penalty = min(p.Score, points.Penalty)
	p.Score -= points.Penalty
	p.Floor = nil
	return points
}

func colorIndex(color string) int {
	for i, c := range colors {
		if c == color {
			return i
		}
	}
	panic("invalid tile in a completed pattern line")
}

func placementScore(wall *[5][5]bool, row, column int) int {
	count := func(dr, dc int) int {
		n := 0
		for r, c := row+dr, column+dc; r >= 0 && r < 5 && c >= 0 && c < 5 && wall[r][c]; r, c = r+dr, c+dc {
			n++
		}
		return n
	}
	horizontal := count(0, -1) + count(0, 1)
	vertical := count(-1, 0) + count(1, 0)
	if horizontal == 0 || vertical == 0 {
		return 1 + horizontal + vertical
	}
	// The new tile counts in each connected axis.
	return 2 + horizontal + vertical
}

func (p *Player) wallBonus() Bonus {
	var rows, columns, sets [5]int
	for r, row := range p.Wall {
		for c, filled := range row {
			if filled {
				rows[r]++
				columns[c]++
				sets[(c-r+5)%5]++
			}
		}
	}
	var bonus Bonus
	for i := range rows {
		if rows[i] == 5 {
			bonus.Rows += 2
		}
		if columns[i] == 5 {
			bonus.Columns += 7
		}
		if sets[i] == 5 {
			bonus.Colors += 10
		}
	}
	return bonus
}

func winners(players [2]Player) []int {
	difference := players[0].Score - players[1].Score
	if difference == 0 {
		difference = players[0].Bonus.Rows - players[1].Bonus.Rows
	}
	switch {
	case difference > 0:
		return []int{1}
	case difference < 0:
		return []int{2}
	default:
		return []int{1, 2}
	}
}
