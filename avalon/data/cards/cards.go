package cards

import (
	"avalon/data"
)

type goodCard struct {
}

func (card goodCard) Label() string {
	return "Good"
}

func (card goodCard) AllocatedAsSpy() bool {
	return false
}

func (card goodCard) HasWon(game data.Game) bool {
	return game.State.GoodScore >= 3
}

func (card goodCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": false,
	}
}

func (card goodCard) Reveal(game data.Game) []data.GameReveal {
	return nil
}

func (card goodCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}


type evilCard struct {
}

func (card evilCard) Label() string {
	return "Evil"
}

func (card evilCard) AllocatedAsSpy() bool {
	return true
}

func (card evilCard) HasWon(game data.Game) bool {
	return game.State.EvilScore >= 3
}

func (card evilCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func revealEvil(game data.Game, to data.CardOps) []int {
	players := make([]int, 0)
	for i, role := range game.Roles {
		c := game.Cards[role]
		if c.AllocatedAsSpy() && !c.HiddenFrom(game, to) {
			players = append(players, i)
		}
	}

	return players
}

func (card evilCard) Reveal(game data.Game) []data.GameReveal {
	players := revealEvil(game, card)

	return []data.GameReveal{
		data.GameReveal{
			Label: "These are the evil players",
			Players: players,
		},
	}
}

func (card evilCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}


type cardCtor func() data.CardOps
var CardFactory map[string]cardCtor = map[string]cardCtor{}

func addCtor(f cardCtor) {
	label := f().Label()
	CardFactory[label] = f
}

func init() {
	addCtor(func() data.CardOps { return goodCard{} })
	addCtor(func() data.CardOps { return evilCard{} })
}
