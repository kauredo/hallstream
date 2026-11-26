// Package sport holds each sport's rules: how the clock runs, how many
// periods, when the half falls, timeouts and fouls. The scorer, the state
// machine and the pages read them from here.
package sport

import "encoding/json"

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

	OvertimeLength int    `json:"overtime_length"` // tenths in an overtime period; 0 = the regulation length
	FoulsCarry     bool   `json:"fouls_carry"`     // fouls of the last regulation period run on through overtime
	PeriodWord     string `json:"period_word"`     // "quarter", "half" or "set": which period labels the pages use
	FoulMarks      int    `json:"foul_marks"`      // above 0, the scorer page keeps team fouls; 0 = no fouls record
	FoulPenalty    string `json:"foul_penalty"`    // the on-air word at FoulBonus: "bonus" (basketball) or "free_kick" (futsal)

	Cards       bool `json:"cards"`        // the sport has cards, substitutions, added time and an explicit end of game (soccer)
	StopAtEnd   bool `json:"stop_at_end"`  // a count-up clock stops at the end of the period; false runs on (soccer)
	Suspensions int  `json:"suspensions"`  // tenths a suspension lasts; 0 = the sport has none
	ExtraTime   bool `json:"extra_time"`   // the game can go on past Periods into overtime periods
	StoppageMax int  `json:"stoppage_max"` // the most minutes of added time the scorer can announce; 0 = the sport has none

	MaxPoint      int    `json:"max_point"`       // the most one score action can add: basketball 3, a goal or a set point 1
	Scoring       string `json:"scoring"`         // "points" (basketball) or "sets"
	SetPoints     int    `json:"set_points"`      // points to win a set (25); 0 when Scoring is "points"
	LastSetPoints int    `json:"last_set_points"` // points to win the deciding set (15)
	SetsToWin     int    `json:"sets_to_win"`     // sets to win the match (3 = best of 5)
	WinBy         int    `json:"win_by"`          // lead needed to win a set (2)
}

// Basketball is FIBA.
var Basketball = Profile{ID: "basketball", Periods: 4, Half: 2, Length: 6000, ClockDown: true,
	FoulsReset: true, FoulWarn: 4, FoulBonus: 5, ShotClock: true, FinalHold: 600,
	TimeoutsBy: [3]int{2, 3, 1}, Scoring: "points", MaxPoint: 3, OvertimeLength: 3000, PeriodWord: "quarter",
	FoulMarks: 5, FoulPenalty: "bonus", ExtraTime: true}

// Volleyball: best of 5, sets to 25 (15 in the 5th), win by 2, no clock,
// 2 timeouts per team per set.
var Volleyball = Profile{ID: "volleyball", MaxPoint: 1, Scoring: "sets", SetPoints: 25, LastSetPoints: 15,
	SetsToWin: 3, WinBy: 2, TimeoutsBy: [3]int{2, 2, 2}, PeriodWord: "set"}

// Futsal is FIFA: two halves of 20:00 on a stopped clock, accumulated fouls
// per half (the 6th gives a free kick without a wall) that carry into extra
// time, one timeout per team per half and none in extra time, extra time in
// periods of 5:00.
var Futsal = Profile{ID: "futsal", Periods: 2, Half: 1, Length: 12000, ClockDown: true,
	FoulsReset: true, FoulsCarry: true, FoulWarn: 5, FoulBonus: 6, FinalHold: 600,
	TimeoutsBy: [3]int{1, 1, 0}, Scoring: "points", MaxPoint: 1, OvertimeLength: 3000, PeriodWord: "half",
	FoulMarks: 5, FoulPenalty: "free_kick", ExtraTime: true}

// Handball is IHF: two halves of 30:00 on a clock that counts up, 2-minute
// suspensions, no team fouls, two timeouts per team per half shown (three per
// match is the rule, not enforced), extra time in periods of 5:00.
var Handball = Profile{ID: "handball", Periods: 2, Half: 1, Length: 18000, ClockDown: false,
	StopAtEnd: true, FinalHold: 600, TimeoutsBy: [3]int{2, 2, 0}, Scoring: "points", MaxPoint: 1,
	OvertimeLength: 3000, PeriodWord: "half", Suspensions: 1200, ExtraTime: true}

// Soccer is IFAB: two halves of 45:00 on a clock that counts up and runs on
// into added time, cards and substitutions per player, no fouls, timeouts or
// suspensions, no extra time. The game ends on the scorer's "Fim do jogo", so
// FinalHold is 0.
var Soccer = Profile{ID: "soccer", Periods: 2, Half: 1, Length: 27000, ClockDown: false,
	StopAtEnd: false, FinalHold: 0, TimeoutsBy: [3]int{0, 0, 0}, Scoring: "points", MaxPoint: 1,
	PeriodWord: "half", Cards: true, StoppageMax: 15}

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
	Futsal.ID:     Futsal,
	Handball.ID:   Handball,
	Soccer.ID:     Soccer,
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

// Overtime reports whether period is an overtime (after Periods).
func (p Profile) Overtime(period int) bool { return period > p.Periods }

// ResetsFouls reports whether starting period puts the team fouls back to 0:
// not when the sport never resets them, and not in an overtime when the
// fouls carry on from the last regulation period.
func (p Profile) ResetsFouls(period int) bool {
	if !p.FoulsReset {
		return false
	}
	return !(p.FoulsCarry && p.Overtime(period))
}

// CountUp reports whether the clock counts up (handball, soccer); false for a
// countdown and for a sport with no clock (volleyball).
func (p Profile) CountUp() bool { return !p.ClockDown && p.Length > 0 }

// length is the length of period: an overtime's, or the regulation one.
func (p Profile) length(period, length int) int {
	if p.Overtime(period) && p.OvertimeLength > 0 {
		return p.OvertimeLength
	}
	return length
}

// Start is the clock at the start of period, given the scorer's regulation
// length. Countdown: length, or OvertimeLength in an overtime when set.
// Count-up: the time played before period (handball's period 2 starts at 30:00).
func (p Profile) Start(period, length int) int {
	if !p.CountUp() {
		return p.length(period, length)
	}
	t := 0
	for i := 1; i < period; i++ {
		t += p.length(i, length)
	}
	return t
}

// End is where period ends on a count-up clock (its Start plus its length);
// 0 for a countdown.
func (p Profile) End(period, length int) int {
	if !p.CountUp() {
		return 0
	}
	return p.Start(period, length) + p.length(period, length)
}

// MarshalJSON adds count_up, which the pages read as game.rules.count_up. It is
// worked out from CountUp, so no profile can say one thing and do another.
func (p Profile) MarshalJSON() ([]byte, error) {
	type plain Profile
	return json.Marshal(struct {
		plain
		CountUp bool `json:"count_up"`
	}{plain(p), p.CountUp()})
}
