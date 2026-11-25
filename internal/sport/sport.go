// Package sport holds each sport's rules: how the clock runs, how many
// periods, when the half falls, timeouts and fouls. The scorer, the state
// machine and the pages read them from here.
package sport

// Profile is one sport's rules. Times are in tenths of a second. It goes to
// the pages as game.rules, so the JSON names are part of the contract.
type Profile struct {
	ID         string `json:"id"`          // as in club.Sports
	Periods    int    `json:"periods"`     // regulation periods
	Half       int    `json:"half"`        // the period after which the half-time break falls
	Length     int    `json:"length"`      // tenths in a regulation period
	ClockDown  bool   `json:"clock_down"`  // the clock counts down
	FoulsReset bool   `json:"fouls_reset"` // team fouls go back to 0 each period
	FoulWarn   int    `json:"foul_warn"`   // fouls at which the pips turn hot; 0 = never
	FoulBonus  int    `json:"foul_bonus"`  // fouls from which the other side shoots; 0 = never
	ShotClock  bool   `json:"shot_clock"`  // the sport has a shot clock
	FinalHold  int    `json:"final_hold"`  // tenths the clock sits at 0 in the last period before the game is over
	TimeoutsBy [3]int `json:"timeouts"`    // timeouts per team: first half, second half, each overtime

	Scoring       string `json:"scoring"`         // "points" (basketball) or "sets"
	SetPoints     int    `json:"set_points"`      // points to win a set (25); 0 when Scoring is "points"
	LastSetPoints int    `json:"last_set_points"` // points to win the deciding set (15)
	SetsToWin     int    `json:"sets_to_win"`     // sets to win the match (3 = best of 5)
	WinBy         int    `json:"win_by"`          // lead needed to win a set (2)
}

// Basketball is FIBA.
var Basketball = Profile{ID: "basketball", Periods: 4, Half: 2, Length: 6000, ClockDown: true,
	FoulsReset: true, FoulWarn: 4, FoulBonus: 5, ShotClock: true, FinalHold: 600,
	TimeoutsBy: [3]int{2, 3, 1}, Scoring: "points"}

// Volleyball: best of 5, sets to 25 (15 in the 5th), win by 2, no clock,
// 2 timeouts per team per set.
var Volleyball = Profile{ID: "volleyball", Scoring: "sets", SetPoints: 25, LastSetPoints: 15,
	SetsToWin: 3, WinBy: 2, TimeoutsBy: [3]int{2, 2, 2}}

// Get returns the profile for id; an unknown or empty id is basketball.
func Get(id string) Profile {
	if p, ok := profiles[id]; ok {
		return p
	}
	return Basketball
}

var profiles = map[string]Profile{
	Basketball.ID: Basketball,
	Volleyball.ID: Volleyball,
}

// Sets reports whether the sport is scored in sets.
func (p Profile) Sets() bool { return p.Scoring == "sets" }

// SetTarget is the points that win set n (1-based).
func (p Profile) SetTarget(n int) int {
	if n == 2*p.SetsToWin-1 {
		return p.LastSetPoints
	}
	return p.SetPoints
}

// SetWon reports which side has won a set at home-away in set n: "home",
// "away" or "" while it goes on.
func (p Profile) SetWon(n, home, away int) string {
	t := p.SetTarget(n)
	switch {
	case home >= t && home-away >= p.WinBy:
		return "home"
	case away >= t && away-home >= p.WinBy:
		return "away"
	}
	return ""
}

// Timeouts is how many timeouts each team has in period:
// TimeoutsBy[0] up to Half, [1] after Half up to Periods, [2] after.
// A sets sport gives TimeoutsBy[0] in every set.
func (p Profile) Timeouts(period int) int {
	if p.Sets() {
		return p.TimeoutsBy[0]
	}
	switch {
	case period <= p.Half:
		return p.TimeoutsBy[0]
	case period <= p.Periods:
		return p.TimeoutsBy[1]
	}
	return p.TimeoutsBy[2]
}

// ResetsTimeouts reports whether starting period puts timeouts back to 0:
// it is the period after Half, or any period after Periods. A sets sport
// resets them at every new set.
func (p Profile) ResetsTimeouts(period int) bool {
	if p.Sets() {
		return period >= 2
	}
	return period == p.Half+1 || period > p.Periods
}
