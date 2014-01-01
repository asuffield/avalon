package data

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

type GameReveal struct {
	Players []int `json:"players"`
	Label string `json:"label"`
}

type CardOps interface {
	// This returns the card's label
	Label() string
	// This is true if the card takes up a spy slot during game creation
	AllocatedAsSpy() bool
	// Maximum number of copies of this card which may be used in the
	// game (0 for the filler cards)
	Maximum() int
	// This is the priority order for being the one to assassinate Merlin
	AssassinPriority() int
	// This is true if the card has won the game
	HasWon(Game) bool
	// This returns a map of actions to offer, which is true if that
	// action should be allowed at the moment
	PermittedActions(Game, Proposal) map[string]bool
	// This returns some GameReveal objects for this card, at reveal time
	Reveal(Game) []GameReveal
	// This is a utility function for composing Reveal - it is true if
	// this card should not be revealed to the argument
	HiddenFrom(Game, CardOps) bool
}

type Mission struct {
	Size int `json:"size"`
	FailsAllowed int `json:"fails_allowed"`
}

type GameSetup struct {
	Missions []Mission `json:"missions"`
	Cards []string `json:"cards"`
	Spies int `json:"spies"`
}

type Proposal struct {
	Leader int
	Players []int
	Votes []bool
	Voted []bool
}

type GameState struct {
	// This is used to manage data migrations
	DataVersion int

	// These value are updated by a completed proposal
	HaveProposal bool

	// These values are updated by a completed vote (or mission)
	Leader int
	ThisProposal int
	ThisVote int
	HaveActions bool

	// These values are updated by a completed mission
	ThisMission int
	MissionsComplete []bool
	GoodScore int
	EvilScore int
	AssassinTarget int
	GameOver bool
}

type GameStatic struct {
	Id string
	Hangout string
	StartTime time.Time
	Setup GameSetup
	UserIDs []string
	AIs []int
	Roles []int
}

type Game struct {
	GameStatic
	State *GameState
	Cards []CardOps
}

type Actions struct {
	Mission int
	Proposal int
	Actions []bool
	Acted []bool
}

type MissionResult struct {
	Mission int `json:"mission"`
	Proposal int `json:"proposal"`
	Leader int `json:"leader"`
	Players []int `json:"players"`
	Fails int `json:"fails"`
	FailsAllowed int `json:"fails_allowed"`
}

type VoteResult struct {
	Index int `json:"vote_index"`
	Mission int `json:"mission"`
	Proposal int `json:"proposal"`
	Leader int `json:"leader"`
	Players []int `json:"players"`
	Votes []bool `json:"votes"`
}

func (game Game) LookupUserID(userid string) (int, bool) {
	for i, v := range game.UserIDs {
		if v == userid {
			return i, true
		}
	}
	return -1, false
}

func (proposal Proposal) LookupMissionSlot(pos int) (int, bool) {
	for i, v := range proposal.Players {
		if v == pos {
			return i, true
		}
	}
	return -1, false
}

var gameSizes map[int]GameSetup = map[int]GameSetup {
	5:  GameSetup{Spies: 2, Missions: []Mission { Mission{2, 0}, Mission{3, 0}, Mission{2, 0}, Mission{3, 0}, Mission{3, 0} }},
	6:  GameSetup{Spies: 2, Missions: []Mission { Mission{2, 0}, Mission{3, 0}, Mission{4, 0}, Mission{3, 0}, Mission{4, 0} }},
	7:  GameSetup{Spies: 3, Missions: []Mission { Mission{2, 0}, Mission{3, 0}, Mission{3, 0}, Mission{4, 1}, Mission{4, 0} }},
	8:  GameSetup{Spies: 3, Missions: []Mission { Mission{3, 0}, Mission{4, 0}, Mission{4, 0}, Mission{5, 1}, Mission{5, 0} }},
	9:  GameSetup{Spies: 3, Missions: []Mission { Mission{3, 0}, Mission{4, 0}, Mission{4, 0}, Mission{5, 1}, Mission{5, 0} }},
	10: GameSetup{Spies: 4, Missions: []Mission { Mission{3, 0}, Mission{4, 0}, Mission{4, 0}, Mission{5, 1}, Mission{5, 0} }},
}

func GetSizeSetup(players int) GameSetup {
	return gameSizes[players]
}

func (game Game) FindAssassin() int {
	merlin := false
	assassin := 0
	for i, card := range game.Cards {
		if card.Label() == "Merlin" {
			merlin = true
		}
		if card.AssassinPriority() > game.Cards[assassin].AssassinPriority() {
			assassin = i
		}
	}
	if !merlin || game.Cards[assassin].AssassinPriority() == 0 {
		// This game has no assassin
		return -1
	}
	return assassin
}

func MakeGameSetup(players int) GameSetup {
	if (players != 5) {
		panic("Can only handle 5 players right now")
	}

	missions := []Mission { Mission{2, 0}, Mission{3, 0}, Mission{2, 0}, Mission{3, 0}, Mission{3, 0} }
	cards := []string { "Good", "Good", "Good", "Evil", "Evil" }

	return GameSetup{Missions: missions, Cards: cards}
}

func RandomString(length int) (str string) {
	b := make([]byte, length)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func (game GameStatic) Size() int {
	return 1
}

func (proposal Proposal) Size() int {
	return 1
}

func (result MissionResult) Size() int {
	return 1
}

func (result VoteResult) Size() int {
	return 1
}
