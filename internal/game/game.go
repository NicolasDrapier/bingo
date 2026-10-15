package game

import "strconv"

type Size int

const (
	Size3 Size = 3
	Size4 Size = 4
	Size5 Size = 5
)

func (s Size) Valid() bool {
	switch s {
	case Size3, Size4, Size5:
		return true
	}
	return false
}

func (s Size) HasFreeCenter() bool {
	return s%2 == 1
}

func (s Size) Center() (row, col int, ok bool) {
	if !s.HasFreeCenter() {
		return 0, 0, false
	}
	m := int(s) / 2
	return m, m, true
}

func (s Size) Patterns() [][][2]int {
	n := int(s)
	pats := make([][][2]int, 0, n*2+2)
	for i := 0; i < n; i++ {
		row := make([][2]int, n)
		col := make([][2]int, n)
		for j := 0; j < n; j++ {
			row[j] = [2]int{i, j}
			col[j] = [2]int{j, i}
		}
		pats = append(pats, row, col)
	}
	if s.HasFreeCenter() {
		main := make([][2]int, n)
		anti := make([][2]int, n)
		for i := 0; i < n; i++ {
			main[i] = [2]int{i, i}
			anti[i] = [2]int{i, n - 1 - i}
		}
		pats = append(pats, main, anti)
	}
	return pats
}

func KeyPattern(p [][2]int) string {
	n := len(p)

	row0, col0 := p[0][0], p[0][1]
	isRow := true
	for _, c := range p {
		if c[0] != row0 {
			isRow = false
			break
		}
	}
	if isRow {
		return "row:" + strconv.Itoa(row0)
	}

	isCol := true
	for _, c := range p {
		if c[1] != col0 {
			isCol = false
			break
		}
	}
	if isCol {
		return "col:" + strconv.Itoa(col0)
	}

	isMain := true
	for i, c := range p {
		if c[0] != c[1] || c[0] != i {
			isMain = false
			break
		}
	}
	if isMain {
		return "diag:main"
	}

	size := n
	isAnti := true
	for i, c := range p {
		if c[0]+c[1] != size-1 || c[0] != i {
			isAnti = false
			break
		}
	}
	if isAnti {
		return "diag:anti"
	}
	return "unknown"
}

func (s Size) Wins(checked [][]bool, freeCenter bool) []string {
	var out []string
	for _, p := range s.Patterns() {
		if s.patternComplete(p, checked, freeCenter) {
			out = append(out, KeyPattern(p))
		}
	}
	return out
}

func (s Size) patternComplete(p [][2]int, checked [][]bool, freeCenter bool) bool {
	cr, cc, isCenter := s.Center()
	for _, c := range p {
		if freeCenter && isCenter && c[0] == cr && c[1] == cc {
			continue
		}
		if !checked[c[0]][c[1]] {
			return false
		}
	}
	return true
}
