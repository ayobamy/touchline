package events

// Player is one member of a squad. All names are fictional.
type Player struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Number   int    `json:"number"`
	Position string `json:"position"`
}

// Team is a fictional club and its starting eleven.
type Team struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ShortName string   `json:"short_name"`
	Players   []Player `json:"players"`
}

// Match describes the fixture that an event stream belongs to.
type Match struct {
	ID       string `json:"id"`
	Home     Team   `json:"home"`
	Away     Team   `json:"away"`
	Venue    string `json:"venue"`
	Seed     uint64 `json:"seed"`
	Scenario string `json:"scenario"`
}

// IsHome reports whether teamID is the home team.
func (m Match) IsHome(teamID string) bool { return teamID == m.Home.ID }

// Opponent returns the ID of the team facing teamID.
func (m Match) Opponent(teamID string) string {
	if m.IsHome(teamID) {
		return m.Away.ID
	}
	return m.Home.ID
}

// Message kinds in an event stream.
const (
	KindMatch = "match"
	KindEvent = "event"
)

// Message is one line of the newline-delimited JSON stream. A stream starts
// with exactly one match message followed by event messages in order.
type Message struct {
	Kind  string `json:"kind"`
	Match *Match `json:"match,omitempty"`
	Event *Event `json:"event,omitempty"`
}
