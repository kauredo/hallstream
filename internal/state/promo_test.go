package state

import (
	"testing"
	"time"
)

func TestPromoGoesBack(t *testing.T) {
	m, _ := newTest()
	m.SetScreen("desconto", true)
	m.ShowPromo(PromoClip{ID: "a", End: "back"})
	s := m.Snapshot()
	if s.Screen.Current != "promo" || s.Promo.Previous != "desconto" || s.Promo.Seq != 1 || s.Promo.Clip == nil {
		t.Fatalf("show: %+v %+v", s.Screen, s.Promo)
	}
	// Another clip while one plays keeps the screen to go back to.
	m.ShowPromo(PromoClip{ID: "b", End: "back"})
	if m.EndPromo(1) {
		t.Error("the first clip's end stopped the second")
	}
	if !m.EndPromo(2) {
		t.Fatal("the second clip's end was ignored")
	}
	s = m.Snapshot()
	if s.Screen.Current != "desconto" || s.Promo.Clip != nil {
		t.Fatalf("after end: %+v %+v", s.Screen, s.Promo)
	}
	if m.EndPromo(0) {
		t.Error("stop with no promo changed the screen")
	}
}

func TestPromoLeftByHand(t *testing.T) {
	m, _ := newTest()
	m.ShowPromo(PromoClip{ID: "a"})
	m.SetScreen("intervalo", true)
	if s := m.Snapshot(); s.Promo.Clip != nil || m.EndPromo(0) {
		t.Fatalf("clip kept after leaving: %+v", s.Promo)
	}
}

func TestPromoEndsAfterItsLength(t *testing.T) {
	m, c := newTest()
	m.ShowPromo(PromoClip{ID: "a", End: "back", Duration: 10})
	c.t = c.t.Add(10*time.Second + PromoGrace)
	m.Tick()
	if m.Snapshot().Screen.Current != "promo" {
		t.Fatal("ended before the grace")
	}
	c.t = c.t.Add(time.Second)
	m.Tick()
	if cur := m.Snapshot().Screen.Current; cur != "marcador" {
		t.Fatalf("screen %q, want marcador", cur)
	}

	m.ShowPromo(PromoClip{ID: "a", End: "back"}) // not measured
	c.t = c.t.Add(PromoUnknown)
	m.Tick()
	if m.Snapshot().Screen.Current != "promo" {
		t.Fatal("an unmeasured clip ended early")
	}
	c.t = c.t.Add(PromoGrace + time.Second)
	m.Tick()
	if cur := m.Snapshot().Screen.Current; cur != "marcador" {
		t.Fatalf("unmeasured clip stuck on %q", cur)
	}

	m.ShowPromo(PromoClip{ID: "a", End: "loop", Duration: 10})
	c.t = c.t.Add(time.Minute)
	m.Tick()
	if m.Snapshot().Screen.Current != "promo" {
		t.Fatal("a looping clip ended")
	}
}

func TestSyncPromo(t *testing.T) {
	m, _ := newTest()
	m.SetScreen("plantel", true)
	m.ShowPromo(PromoClip{ID: "a", Headline: "Velho"})
	lib := map[string]PromoClip{"a": {ID: "a", Headline: "Novo"}}
	get := func(id string) (PromoClip, bool) { c, ok := lib[id]; return c, ok }
	mini := &PromoClip{ID: "m"}
	m.SyncPromo(get, mini)
	s := m.Snapshot()
	if s.Promo.Clip.Headline != "Novo" || s.Promo.Mini == nil || s.Promo.Mini.ID != "m" {
		t.Fatalf("sync: %+v", s.Promo)
	}
	delete(lib, "a")
	m.SyncPromo(get, nil)
	s = m.Snapshot()
	if s.Screen.Current != "plantel" || s.Promo.Clip != nil || s.Promo.Mini != nil {
		t.Fatalf("deleted clip: %+v %+v", s.Screen, s.Promo)
	}
}

func TestPromoIsNotAScreenButton(t *testing.T) {
	m, _ := newTest()
	if m.SetScreen("promo", true) == nil {
		t.Error("promo without a clip was accepted")
	}
}
