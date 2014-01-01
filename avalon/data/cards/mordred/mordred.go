package mordred

import (
	"avalon/data"
	"avalon/data/cards"
)

type mordredCard struct {
}

func (card mordredCard) Label() string {
	return "Mordred"
}

func (card mordredCard) AllocatedAsSpy() bool {
	return true
}

func (card mordredCard) Maximum() int {
	return 1
}

func (card mordredCard) AssassinPriority() int {
	return 2
}

func (card mordredCard) HasWon(game data.Game) bool {
	return !cards.GoodHasWon(game)
}

func (card mordredCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func (card mordredCard) Reveal(game data.Game) []data.GameReveal {
	return []data.GameReveal{ cards.RevealEvil(game, card) }
}

func (card mordredCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	if other.Label() == "Merlin" {
		return true
	}
	return false
}

func init() {
	cards.AddCardType(mordredCard{})
}
