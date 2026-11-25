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

func TestResetsTimeouts(t *testing.T) {
	for p, want := range map[int]bool{1: false, 2: false, 3: true, 4: false, 5: true, 6: true} {
		if got := Basketball.ResetsTimeouts(p); got != want {
			t.Errorf("ResetsTimeouts(%d) = %v, want %v", p, got, want)
		}
	}
}
