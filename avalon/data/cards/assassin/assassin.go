package assassin

import (
	"avalon/data"
	"avalon/data/cards"
)

type assassinCard struct {
}

func (card assassinCard) Label() string {
	return "Assassin"
}

func (card assassinCard) AllocatedAsSpy() bool {
	return true
}

func (card assassinCard) Maximum() int {
	return 1
}

func (card assassinCard) AssassinPriority() int {
	return 10
}

func (card assassinCard) HasWon(game data.Game) bool {
	return !cards.GoodHasWon(game)
}

func (card assassinCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func (card assassinCard) Reveal(game data.Game) []data.GameReveal {
	return []data.GameReveal{ cards.RevealEvil(game, card) }
}

func (card assassinCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}

func init() {
	cards.AddCardType(assassinCard{})
}
