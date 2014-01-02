package oberon

import (
	"avalon/data"
	"avalon/data/cards"
)

type oberonCard struct {
}

func (card oberonCard) Label() string {
	return "Oberon"
}

func (card oberonCard) AllocatedAsSpy() bool {
	return true
}

func (card oberonCard) Maximum() int {
	return 1
}

func (card oberonCard) AssassinPriority() int {
	return 0
}

func (card oberonCard) HasWon(game data.Game) bool {
	return !cards.GoodHasWon(game)
}

func (card oberonCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func (card oberonCard) Reveal(game data.Game) []data.GameReveal {
	return nil
}

func (card oberonCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return true
}

func init() {
	cards.AddCardType(oberonCard{})
}
