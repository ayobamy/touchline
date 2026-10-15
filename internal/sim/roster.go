package sim

import (
	"fmt"

	"github.com/ayobamy/touchline/internal/events"
)

// slot is a position in a 4-3-3 and its base zone, in the attacking frame of
// its own team: a is metres from the team's own goal line, y is across the
// pitch.
type slot struct {
	position string
	a, y     float64
}

var formation = []slot{
	{"GK", 5, 34},
	{"RB", 30, 58}, {"CB", 22, 42}, {"CB", 22, 26}, {"LB", 30, 10},
	{"CM", 45, 34}, {"CM", 52, 50}, {"CM", 52, 18},
	{"RW", 75, 60}, {"ST", 82, 34}, {"LW", 75, 8},
}

// Index of the striker in formation; they take kick-offs.
const strikerIndex = 9

type rosterEntry struct {
	number int
	name   string
}

// Fictional clubs and players. Any resemblance to real people is unintended.
var (
	homeTeam = teamSpec{
		id: "riv", name: "Riverside Athletic", short: "RIV",
		players: []rosterEntry{
			{1, "Oskar Brandt"}, {2, "Kwame Ashby"}, {4, "Dario Castell"}, {5, "Idris Whitlock"},
			{3, "Felix Nwosu"}, {6, "Joon Moreau"}, {8, "Emeka Varga"}, {10, "Luca Adeyemi"},
			{7, "Sami Ferreira"}, {9, "Tunde Quinlan"}, {11, "Rafael Hale"},
		},
	}
	awayTeam = teamSpec{
		id: "iro", name: "Ironbridge Rovers", short: "IRO",
		players: []rosterEntry{
			{1, "Hugo Petrov"}, {2, "Malik Duarte"}, {4, "Callum Osei"}, {5, "Arjun Kerr"},
			{3, "Noah Ibarra"}, {6, "Yusuf Sato"}, {8, "Mateo Rahman"}, {10, "Ade Novak"},
			{7, "Leo Mensah"}, {9, "Ivan Okoro"}, {11, "Theo Bramwell"},
		},
	}
)

type teamSpec struct {
	id, name, short string
	players         []rosterEntry
}

func (t teamSpec) build() events.Team {
	team := events.Team{ID: t.id, Name: t.name, ShortName: t.short}
	for i, p := range t.players {
		team.Players = append(team.Players, events.Player{
			ID:       fmt.Sprintf("%s-%02d", t.id, p.number),
			Name:     p.name,
			Number:   p.number,
			Position: formation[i].position,
		})
	}
	return team
}
