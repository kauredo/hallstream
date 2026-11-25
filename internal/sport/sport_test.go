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
