package percival

import (
	"avalon/data"
	"avalon/data/cards"
)

type percivalCard struct {
}

func (card percivalCard) Label() string {
	return "Percival"
}

func (card percivalCard) AllocatedAsSpy() bool {
	return false
}

func (card percivalCard) Maximum() int {
	return 1
}

func (card percivalCard) AssassinPriority() int {
	return 0
}

func (card percivalCard) HasWon(game data.Game) bool {
	return cards.GoodHasWon(game)
}

func (card percivalCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": false,
	}
}

func (card percivalCard) Reveal(game data.Game) []data.GameReveal {
	merlin := -1
	morgana := -1
	for i, role := range game.Roles {
		c := game.Cards[role]
		if c.Label() == "Merlin" {
			merlin = i
		} else if c.Label() == "Morgana" {
			morgana = i
		}
	}

	if merlin == -1 {
		return []data.GameReveal{ }
	} else if morgana == -1 {
		return []data.GameReveal{ data.GameReveal{ Label: "This is Merlin", Players: []int{ merlin } } }
	} else {
		return []data.GameReveal{ data.GameReveal{ Label: "This is Merlin and Morgana", Players: []int{ merlin, morgana } } }
	}
}

func (card percivalCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}

func init() {
	cards.AddCardType(percivalCard{})
}
