package cards

import (
	"avalon/data"
	"reflect"
	"strings"
)

type goodCard struct {
}

func (card goodCard) Label() string {
	return "Good"
}

func (card goodCard) AllocatedAsSpy() bool {
	return false
}

func (card goodCard) Maximum() int {
	return 0
}

func (card goodCard) AssassinPriority() int {
	return 0
}

func (card goodCard) HasWon(game data.Game) bool {
	return GoodHasWon(game)
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

func (card evilCard) Maximum() int {
	return 0
}

func (card evilCard) AssassinPriority() int {
	return 0
}

func (card evilCard) HasWon(game data.Game) bool {
	return !GoodHasWon(game)
}

func (card evilCard) PermittedActions(game data.Game, proposal data.Proposal) map[string]bool {
	return map[string]bool {
		"Success": true,
		"Failure": true,
	}
}

func (card evilCard) Reveal(game data.Game) []data.GameReveal {
	return []data.GameReveal{ RevealEvil(game, card) }
}

func (card evilCard) HiddenFrom(game data.Game, other data.CardOps) bool {
	return false
}


func GoodHasWon(game data.Game) bool {
	if game.State.AssassinTarget != -1 && game.Cards[game.State.AssassinTarget].Label() == "Merlin" {
		// Merlin has been assassinated
		return false
	}
	return game.State.GoodScore >= 3
}

func RevealEvil(game data.Game, to data.CardOps) data.GameReveal {
	players := make([]int, 0)
	hiddenEvil := make([]data.CardOps, 0)
	for i, role := range game.Roles {
		c := game.Cards[role]
		if c.AllocatedAsSpy() {
			if !c.HiddenFrom(game, to) {
				players = append(players, i)
			} else {
				hiddenEvil = append(hiddenEvil, c)
			}
		}
	}

	label := "These are the evil players"
	if len(hiddenEvil) > 0 {
		hiddenLabels := make([]string, len(hiddenEvil))
		for i, c := range hiddenEvil {
			hiddenLabels[i] = c.Label()
		}
		label = label + " (excluding " + strings.Join(hiddenLabels, ", ") + ")"
	}

	return data.GameReveal{
		Label: label,
		Players: players,
	}
}

type cardCtor func() data.CardOps
var CardFactory map[string]cardCtor = map[string]cardCtor{}

func addCardCtor(f cardCtor) {
	label := f().Label()
	CardFactory[label] = f
}

// Reflection is a little overkill here, but go lacks good generic programming features...
func AddCardType(c data.CardOps) {
	ty := reflect.TypeOf(c)
	addCardCtor(func() data.CardOps { return reflect.New(ty).Interface().(data.CardOps) })
}

func AllCards() []data.CardOps {
	cards := []data.CardOps{}
	for _, ctor := range CardFactory {
		cards = append(cards, ctor())
	}
	return cards
}

func init() {
	AddCardType(goodCard{})
	AddCardType(evilCard{})
}
