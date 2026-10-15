package game

import (
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func grid(size int, cells ...bool) [][]bool {
	g := make([][]bool, size)
	for i := range g {
		g[i] = make([]bool, size)
	}
	for i, v := range cells {
		g[i/size][i%size] = v
	}
	return g
}

func allTrue(n int) []bool {
	b := make([]bool, n)
	for i := range b {
		b[i] = true
	}
	return b
}

func TestWinsAllChecked(t *testing.T) {
	cases := []struct {
		size Size
		want int
	}{
		{Size3, 8},
		{Size4, 8},
		{Size5, 12},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(int(tc.size)), func(t *testing.T) {
			g := make([][]bool, int(tc.size))
			for i := range g {
				g[i] = make([]bool, int(tc.size))
				for j := range g[i] {
					g[i][j] = true
				}
			}
			got := tc.size.Wins(g, false)
			if len(got) != tc.want {
				t.Fatalf("size %d: got %d wins %v, want %d", tc.size, len(got), got, tc.want)
			}
		})
	}
}

func TestWinsNoneChecked(t *testing.T) {
	for _, s := range []Size{Size3, Size4, Size5} {
		g := make([][]bool, int(s))
		for i := range g {
			g[i] = make([]bool, int(s))
		}
		if w := s.Wins(g, false); len(w) != 0 {
			t.Fatalf("size %d: expected no win, got %v", s, w)
		}
	}
}

func TestWinsSingleRow(t *testing.T) {

	g := grid(3,
		false, false, false,
		true, true, true,
		false, false, false,
	)
	got := Size3.Wins(g, false)
	if !reflect.DeepEqual(got, []string{"row:1"}) {
		t.Fatalf("got %v, want [row:1]", got)
	}
}

func TestWinsSingleCol(t *testing.T) {
	g := grid(4,
		false, true, false, false,
		false, true, false, false,
		false, true, false, false,
		false, true, false, false,
	)
	got := Size4.Wins(g, false)
	if !reflect.DeepEqual(got, []string{"col:1"}) {
		t.Fatalf("got %v, want [col:1]", got)
	}
}

func TestWinsDiagonals(t *testing.T) {

	g := grid(3,
		true, false, false,
		false, true, false,
		false, false, true,
	)
	got := Size3.Wins(g, false)
	if !reflect.DeepEqual(got, []string{"diag:main"}) {
		t.Fatalf("got %v, want [diag:main]", got)
	}

	g = grid(3,
		false, false, true,
		false, true, false,
		true, false, false,
	)
	got = Size3.Wins(g, false)
	if !reflect.DeepEqual(got, []string{"diag:anti"}) {
		t.Fatalf("got %v, want [diag:anti]", got)
	}
}

func TestWinsFreeCenter(t *testing.T) {

	g := grid(3,
		true, true, true,
		true, false, true,
		true, true, true,
	)
	got := Size3.Wins(g, true)
	sort.Strings(got)
	want := []string{"col:0", "col:1", "col:2", "diag:anti", "diag:main", "row:0", "row:1", "row:2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	got = Size3.Wins(g, false)
	sort.Strings(got)
	want = []string{"col:0", "col:2", "row:0", "row:2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWinsFreeCenter5x5(t *testing.T) {

	cells := allTrue(25)
	cells[12] = false
	g := grid(5, cells...)
	got := Size5.Wins(g, true)
	if len(got) != 12 {
		t.Fatalf("got %d wins %v, want 12", len(got), got)
	}

	got = Size5.Wins(g, false)
	if len(got) != 8 {
		t.Fatalf("got %d wins %v, want 8", len(got), got)
	}
}

func TestCenter(t *testing.T) {
	if _, _, ok := Size4.Center(); ok {
		t.Fatal("4x4 ne doit pas avoir de centre")
	}
	r, c, ok := Size5.Center()
	if !ok || r != 2 || c != 2 {
		t.Fatalf("5x5 centre = (%d,%d) ok=%v, want (2,2,true)", r, c, ok)
	}
}

func TestKeyPattern(t *testing.T) {
	cases := []struct {
		p    [][2]int
		want string
	}{
		{[][2]int{{1, 0}, {1, 1}, {1, 2}}, "row:1"},
		{[][2]int{{0, 2}, {1, 2}, {2, 2}}, "col:2"},
		{[][2]int{{0, 0}, {1, 1}, {2, 2}}, "diag:main"},
		{[][2]int{{0, 2}, {1, 1}, {2, 0}}, "diag:anti"},
	}
	for _, tc := range cases {
		if got := KeyPattern(tc.p); got != tc.want {
			t.Errorf("KeyPattern(%v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestPatternsCount(t *testing.T) {
	cases := map[Size]int{Size3: 8, Size4: 8, Size5: 12}
	for s, want := range cases {
		if got := len(s.Patterns()); got != want {
			t.Errorf("size %d: %d patterns, want %d", s, got, want)
		}
	}
}
