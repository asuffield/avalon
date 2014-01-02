package morgana

import (
	"avalon/data"
	"avalon/data/cards"
)

type morganaCard struct {
}

func (card morganaCard) Label() string {
	return "Morgana"
}

func (card morganaCard) AllocatedAsSpy() bool {
	return true
}

func (card morganaCard) Maximum() int {
	return 1
}

func (card morganaCard) AssassinPriority() int {
	return 1
}

func (card morganaCard) HasWon(game data.Game) bool {
	return !cards.GoodHasWon(game)
}

func (card morganaCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func (card morganaCard) Reveal(game data.Game) []data.GameReveal {
	return []data.GameReveal{ cards.RevealEvil(game, card) }
}

func (card morganaCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}

func init() {
	cards.AddCardType(morganaCard{})
}
