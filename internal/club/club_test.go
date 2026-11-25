package club

import "testing"

func ratio(t *testing.T, a, b string) float64 {
	t.Helper()
	x, ok := parse(a)
	y, ok2 := parse(b)
	if !ok || !ok2 {
		t.Fatalf("not colours: %q %q", a, b)
	}
	return contrast(x, y)
}

func TestTheme(t *testing.T) {
	for _, tt := range []struct {
		name, primary, secondary string
		onRed                    string // "white" or "dark"
	}{
		{"defaults", DefaultPrimary, DefaultSecondary, "white"},
		{"red and navy", "#BD1522", "#181841", "white"},
		{"light primary", "#F4C430", "#0B6E4F", "dark"},
		{"dark primary", "#0B1F4B", "#C8102E", "white"},
		{"white secondary", "#1D7A3A", "#FFFFFF", "white"},
		{"grey primary", "#808080", "#808080", "dark"},
		{"orange", "#E5482A", "#1C1A17", "dark"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			th := Theme(tt.primary, tt.secondary)
			for _, k := range []string{"red", "on-red", "kit", "on-kit", "blood", "ink", "navy", "red-hi", "red-lo", "red-text", "tint", "tint-dim"} {
				if _, ok := parse(th[k]); !ok {
					t.Fatalf("%s = %q is not a colour", k, th[k])
				}
			}
			if want, _ := Hex(tt.primary); th["red"] != want {
				t.Errorf("red = %s, want the primary %s", th["red"], want)
			}
			if got := th["on-red"] == "#FFFFFF"; got != (tt.onRed == "white") {
				t.Errorf("on-red = %s, want %s text", th["on-red"], tt.onRed)
			}
			for _, c := range []struct {
				fg, bg string
				min    float64
			}{
				{"on-red", "red", 4.5},
				{"on-kit", "kit", 4.5},
				{"red-hi", "ink", 5.5},
				{"red-lo", "navy", 5.5},
				{"red-text", "#FFFFFF", 4.5},
				{"#FFFFFF", "blood", 7},
				{"#FFFFFF", "navy", 12},
				{"navy", "#FFFFFF", 12},
			} {
				fg, bg := c.fg, c.bg
				if th[fg] != "" {
					fg = th[fg]
				}
				if th[bg] != "" {
					bg = th[bg]
				}
				if r := ratio(t, fg, bg); r < c.min {
					t.Errorf("%s on %s: %s on %s is %.2f:1, want %.1f", c.fg, c.bg, fg, bg, r, c.min)
				}
			}
			for _, k := range []string{"red-rgb", "hi-rgb", "ink-rgb"} {
				if th[k] == "" {
					t.Errorf("%s missing", k)
				}
			}
		})
	}
}

// The tone tells the styles which way to step; the lifted tones stay the
// club's hue, sit above it, and keep their text readable.
func TestThemeToneAndLift(t *testing.T) {
	for _, tt := range []struct{ primary, tone string }{
		{DefaultPrimary, "mid"}, {"#BD1522", "mid"}, {"#0B1F4B", "dark"}, {"#101010", "dark"},
		{"#F4C430", "light"}, {"#E5482A", "light"},
	} {
		th := Theme(tt.primary, DefaultSecondary)
		if th["tone"] != tt.tone {
			t.Errorf("%s: tone %q, want %q", tt.primary, th["tone"], tt.tone)
		}
		p, _ := parse(tt.primary)
		for _, k := range []string{"lift", "lift-2"} {
			c, ok := parse(th[k])
			if !ok {
				t.Fatalf("%s: %s = %q", tt.primary, k, th[k])
			}
			if c.lum() < p.lum() {
				t.Errorf("%s: %s %s is darker than the club colour", tt.primary, k, th[k])
			}
			txt, _ := parse(th["on-"+k])
			if r := contrast(c, txt); r < 4.5 {
				t.Errorf("%s: text on %s is %.1f:1", tt.primary, k, r)
			}
		}
	}
}

func TestThemeKeepsAColourThatAlreadyWorks(t *testing.T) {
	th := Theme("#BD1522", "#181841")
	if th["navy"] != "#181841" || th["red-text"] != "#BD1522" {
		t.Errorf("navy %s, red-text %s: both pass already and should stay", th["navy"], th["red-text"])
	}
}

func TestThemeFallsBackOnBadColours(t *testing.T) {
	bad, good := Theme("vermelho", ""), Theme(DefaultPrimary, DefaultSecondary)
	for k, v := range good {
		if bad[k] != v {
			t.Errorf("%s = %s, want the default %s", k, bad[k], v)
		}
	}
}

func TestHex(t *testing.T) {
	for _, tt := range []struct {
		in, want string
		ok       bool
	}{
		{"#bd1522", "#BD1522", true},
		{"#F00", "#FF0000", true},
		{" #181841 ", "#181841", true},
		{"181841", "", false},
		{"#18184", "", false},
		{"#GG0000", "", false},
		{"", "", false},
	} {
		got, ok := Hex(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Hex(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSports(t *testing.T) {
	sp, ok := SportOf("basketball")
	if !ok || !sp.Active || sp.Name != "BASQUETEBOL" || sp.Short != "BASKET" {
		t.Errorf("basketball: %+v %v", sp, ok)
	}
	for id, name := range map[string]string{"futsal": "FUTSAL", "handball": "ANDEBOL", "soccer": "FUTEBOL"} {
		if sp, ok := SportOf(id); !ok || sp.Active || sp.Name != name {
			t.Errorf("%s: %+v %v (listed, not active yet)", id, sp, ok)
		}
	}
	if sp, ok := SportOf("volleyball"); !ok || !sp.Active || sp.Name != "VOLEIBOL" {
		t.Errorf("volleyball: %+v %v", sp, ok)
	}
	if _, ok := SportOf("curling"); ok {
		t.Error("an unknown sport was found")
	}
}

func TestZone(t *testing.T) {
	for _, c := range []struct{ zone, country, want string }{
		{"", "ES", "Europe/Madrid"},
		{"", "pt", "Europe/Lisbon"},
		{"America/Manaus", "BR", "America/Manaus"},
		{"", "BR", "America/Sao_Paulo"},
		{"Not/AZone", "XX", "Europe/Lisbon"},
		{"", "", "Europe/Lisbon"},
	} {
		if got := Zone(c.zone, c.country).String(); got != c.want {
			t.Errorf("Zone(%q, %q) = %s, want %s", c.zone, c.country, got, c.want)
		}
	}
}
