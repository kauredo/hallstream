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
	ClockDown  bool   `json:"clock_down"`  // the clock counts down (plans 006 and 007 add count-up sports)
	FoulsReset bool   `json:"fouls_reset"` // team fouls go back to 0 each period
	FoulWarn   int    `json:"foul_warn"`   // fouls at which the pips turn hot; 0 = never
	FoulBonus  int    `json:"foul_bonus"`  // fouls from which the other side shoots; 0 = never
	ShotClock  bool   `json:"shot_clock"`  // the sport has a shot clock
	FinalHold  int    `json:"final_hold"`  // tenths the clock sits at 0 in the last period before the game is over
	TimeoutsBy [3]int `json:"timeouts"`    // timeouts per team: first half, second half, each overtime
}

// Basketball is FIBA.
var Basketball = Profile{ID: "basketball", Periods: 4, Half: 2, Length: 6000, ClockDown: true,
	FoulsReset: true, FoulWarn: 4, FoulBonus: 5, ShotClock: true, FinalHold: 600,
	TimeoutsBy: [3]int{2, 3, 1}}

// Get returns the profile for id; an unknown or empty id is basketball.
func Get(id string) Profile {
	return Basketball
}

// Timeouts is how many timeouts each team has in period p:
// TimeoutsBy[0] for p <= Half, [1] for Half < p <= Periods, [2] after.
func (p Profile) Timeouts(period int) int {
	switch {
	case period <= p.Half:
		return p.TimeoutsBy[0]
	case period <= p.Periods:
		return p.TimeoutsBy[1]
	}
	return p.TimeoutsBy[2]
}

// ResetsTimeouts reports whether starting period p puts timeouts back to 0:
// p == Half+1, and every p > Periods.
func (p Profile) ResetsTimeouts(period int) bool {
	return period == p.Half+1 || period > p.Periods
}
