package sport

import "testing"

func TestGetDefaultsToBasketball(t *testing.T) {
	for _, id := range []string{"", "nonsense", "basketball"} {
		if Get(id) != Basketball {
			t.Errorf("Get(%q) is not basketball", id)
		}
	}
}

func TestTimeouts(t *testing.T) {
	want := []int{2, 2, 3, 3, 1, 1, 1}
	for i, w := range want {
		if got := Basketball.Timeouts(i + 1); got != w {
			t.Errorf("Timeouts(%d) = %d, want %d", i+1, got, w)
		}
	}
}

func TestVolleyballSets(t *testing.T) {
	v := Volleyball
	for n := 1; n <= 4; n++ {
		if got := v.SetTarget(n); got != 25 {
			t.Errorf("SetTarget(%d) = %d, want 25", n, got)
		}
	}
	if got := v.SetTarget(5); got != 15 {
		t.Errorf("SetTarget(5) = %d, want 15", got)
	}
	for _, c := range []struct {
		n, h, a int
		want    string
	}{
		{1, 25, 23, "home"}, {1, 25, 24, ""}, {1, 26, 28, "away"},
		{5, 15, 13, "home"}, {5, 15, 14, ""}, {1, 24, 0, ""},
	} {
		if got := v.SetWon(c.n, c.h, c.a); got != c.want {
			t.Errorf("SetWon(%d, %d, %d) = %q, want %q", c.n, c.h, c.a, got, c.want)
		}
	}
}

func TestVolleyballTimeouts(t *testing.T) {
	for p := 2; p <= 5; p++ {
		if !Volleyball.ResetsTimeouts(p) {
			t.Errorf("ResetsTimeouts(%d) is false", p)
		}
	}
	for p := 1; p <= 5; p++ {
		if got := Volleyball.Timeouts(p); got != 2 {
			t.Errorf("Timeouts(%d) = %d, want 2", p, got)
		}
	}
	if Get("volleyball") != Volleyball || !Get("volleyball").Sets() || Basketball.Sets() {
		t.Error("Get or Sets is wrong")
	}
}

func TestResetsTimeouts(t *testing.T) {
	for p, want := range map[int]bool{1: false, 2: false, 3: true, 4: false, 5: true, 6: true} {
		if got := Basketball.ResetsTimeouts(p); got != want {
			t.Errorf("ResetsTimeouts(%d) = %v, want %v", p, got, want)
		}
	}
}

func TestFutsal(t *testing.T) {
	if Get("futsal") != Futsal {
		t.Error("Get(futsal) is not futsal")
	}
	for p, want := range map[int]bool{1: false, 2: false, 3: true, 4: true} {
		if got := Futsal.Overtime(p); got != want {
			t.Errorf("futsal Overtime(%d) = %v, want %v", p, got, want)
		}
	}
	for p, want := range map[int]bool{1: false, 4: false, 5: true, 6: true} {
		if got := Basketball.Overtime(p); got != want {
			t.Errorf("basketball Overtime(%d) = %v, want %v", p, got, want)
		}
	}
	for i, w := range []int{1, 1, 0, 0} {
		if got := Futsal.Timeouts(i + 1); got != w {
			t.Errorf("futsal Timeouts(%d) = %d, want %d", i+1, got, w)
		}
	}
}

func TestResetsFouls(t *testing.T) {
	for p, want := range map[int]bool{2: true, 3: false, 4: false} {
		if got := Futsal.ResetsFouls(p); got != want {
			t.Errorf("futsal ResetsFouls(%d) = %v, want %v", p, got, want)
		}
	}
	for p := 2; p <= 6; p++ {
		if !Basketball.ResetsFouls(p) {
			t.Errorf("basketball ResetsFouls(%d) is false", p)
		}
	}
	if Volleyball.ResetsFouls(2) {
		t.Error("volleyball resets fouls")
	}
}

func TestMaxPoint(t *testing.T) {
	for _, c := range []struct {
		p    Profile
		want int
	}{{Basketball, 3}, {Futsal, 1}, {Volleyball, 1}} {
		if c.p.MaxPoint != c.want {
			t.Errorf("%s MaxPoint %d, want %d", c.p.ID, c.p.MaxPoint, c.want)
		}
	}
}

func TestCountsUpMatchesCountUp(t *testing.T) {
	for id, p := range profiles {
		if p.CountsUp != p.CountUp() {
			t.Errorf("%s: CountsUp %v, CountUp() %v", id, p.CountsUp, p.CountUp())
		}
	}
	if !Handball.CountUp() || Basketball.CountUp() || Volleyball.CountUp() || Futsal.CountUp() {
		t.Error("only handball counts up")
	}
	if Get("handball") != Handball {
		t.Error("Get(handball) is not Handball")
	}
}

func TestHandballStartAndEnd(t *testing.T) {
	h := Handball
	for _, c := range []struct{ p, start, end int }{{1, 0, 18000}, {2, 18000, 36000}, {3, 36000, 39000}, {4, 39000, 42000}} {
		if got := h.Start(c.p, 18000); got != c.start {
			t.Errorf("Start(%d) = %d, want %d", c.p, got, c.start)
		}
		if got := h.End(c.p, 18000); got != c.end {
			t.Errorf("End(%d) = %d, want %d", c.p, got, c.end)
		}
	}
	if h.Start(2, 15000) != 15000 || h.End(2, 15000) != 30000 {
		t.Error("a 25 minute half does not move the second half's start and end")
	}
}

func TestCountdownStartAndEnd(t *testing.T) {
	for p := 1; p <= 4; p++ {
		if got := Basketball.Start(p, 6000); got != 6000 {
			t.Errorf("Start(%d) = %d, want 6000", p, got)
		}
	}
	if got := Basketball.Start(5, 6000); got != 3000 {
		t.Errorf("Start(5) = %d, want 3000", got)
	}
	for p := 1; p <= 5; p++ {
		if got := Basketball.End(p, 6000); got != 0 {
			t.Errorf("End(%d) = %d, want 0", p, got)
		}
	}
}
