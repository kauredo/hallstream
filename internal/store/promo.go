package store

import (
	"os"
	"path/filepath"
)

// Clip is a club video in dados/promo, with the text the overlay shows next to it.
type Clip struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File string `json:"file"` // <id>.mp4 in dados/promo
	Size int64  `json:"size"`
	// Orientation is "vertical" or "horizontal", measured by the panel after
	// the upload; "" until then.
	Orientation string  `json:"orientation"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Duration    float64 `json:"duration"` // seconds, 0 = not known
	Kicker      string  `json:"kicker"`
	Headline    string  `json:"headline"`
	Line        string  `json:"line"`
	Link        string  `json:"link"` // "" or an http(s) address
	ShowQR      bool    `json:"show_qr"`
	Sound       bool    `json:"sound"`
	Volume      int     `json:"volume"` // 0 to 100
	End         string  `json:"end"`    // "back" (to the screen before) or "loop"
	Added       string  `json:"added"`
}

// Promo is the video library.
type Promo struct {
	Clips []Clip `json:"clips"`
	// TimeoutClip plays small next to the camera on desconto; "" = none.
	TimeoutClip string `json:"timeout_clip"`
}

func (s *Store) PromoDir() string { return filepath.Join(s.Dir, "promo") }

func (s *Store) Promo() Promo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.promo)
}

// UpdatePromo changes the library with fn and saves. If fn fails nothing changes.
func (s *Store) UpdatePromo(fn func(*Promo) error) (Promo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	work := copyOf(s.promo)
	if err := fn(&work); err != nil {
		return Promo{}, err
	}
	if work.Clips == nil {
		work.Clips = []Clip{}
	}
	if err := s.writeJSON("promo.json", work); err != nil {
		return Promo{}, err
	}
	s.promo = work
	return copyOf(work), nil
}

// RemovePromoFile deletes a clip's video file.
func (s *Store) RemovePromoFile(file string) {
	if file != "" && filepath.Base(file) == file {
		os.Remove(filepath.Join(s.PromoDir(), file))
	}
}
