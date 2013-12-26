package data

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

type Mission struct {
	Size int
	FailsAllowed int
}

type Card struct {
	Spy bool
	Label string
}

type GameSetup struct {
	Missions []Mission
	Cards []Card
	Spies int
}

type Proposal struct {
	Leader int
	Players []int
}

type Game struct {
	Id string
	Hangout string
	StartTime time.Time
	Setup GameSetup
	Players []string
	Participants []string
	AIs []int
	Roles []int
	Leader int
	ThisMission int
	ThisProposal int
	LastVoteMission int
	LastVoteProposal int
	GoodScore int
	EvilScore int
	GameOver bool
}

type MissionResult struct {
	Players []int `json:"players"`
	Fails int `json:"fails"`
	FailsAllowed int `json:"fails_allowed"`
}

func (game Game) LookupUserID(userid string) (int, bool) {
	for i, v := range game.Participants {
		if v == userid {
			return i, true
		}
	}
	return -1, false
}

func MakePlayerMap(players []string) map[string]int {
	var playermap = map[string]int {}
	for i, v := range players {
		playermap[v] = i
	}
	return playermap
}

func MakeGameSetup(players int) GameSetup {
	if (players != 5) {
		panic("Can only handle 5 players right now")
	}

	missions := []Mission { Mission{2, 0}, Mission{3, 0}, Mission{2, 0}, Mission{3, 0}, Mission{3, 0} }
	cards := []Card { Card{false, "Good"}, Card{false, "Good"}, Card{false, "Good"}, Card{true, "Evil"}, Card{true, "Evil"} }

	spycount := 0
	for _, card := range cards {
		if card.Spy {
			spycount++
		}
	}

	return GameSetup{Missions: missions, Cards: cards, Spies: spycount}
}

func RandomString(length int) (str string) {
	b := make([]byte, length)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
