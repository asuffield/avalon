package merlin

import (
	"avalon/data"
	"avalon/data/cards"
)

type merlinCard struct {
}

func (card merlinCard) Label() string {
	return "Merlin"
}

func (card merlinCard) AllocatedAsSpy() bool {
	return false
}

func (card merlinCard) Maximum() int {
	return 1
}

func (card merlinCard) AssassinPriority() int {
	return 0
}

func (card merlinCard) HasWon(game data.Game) bool {
	return cards.GoodHasWon(game)
}

func (card merlinCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": false,
	}
}

func (card merlinCard) Reveal(game data.Game) []data.GameReveal {
	return []data.GameReveal{ cards.RevealEvil(game, card) }
}

func (card merlinCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}

func init() {
	cards.AddCardType(merlinCard{})
}
