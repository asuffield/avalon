package state

import (
	"appengine"
	"avalon/data"
	"avalon/db"
	"avalon/web"
	"encoding/json"
	"github.com/gorilla/sessions"
	"net/http"
)

func init() {
	http.Handle("/game/state", web.GameHandler(ReqGameState))
}

type GameState struct {
	Id string `json:"gameid"`
	Players []string `json:"players"`
	State string `json:"state"`
	Leader int `json:"leader"`
	Results []data.MissionResult `json:"mission_results"`
	LastVotes []bool `json:"last_votes"`
	ThisMission int `json:"this_mission"`
	ThisProposal int `json:"this_proposal"`
}

type GameStatePicking struct {
	General GameState `json:"general"`
	MissionSize int `json:"mission_size"`
	MissionFailsAllowed int `json:"mission_fails_allowed"`
}

type GameStateVoting struct {
	General GameState `json:"general"`
	MissionPlayers []string `json:"mission_players"`
}

type GameStateMission struct {
	General GameState `json:"general"`
	MissionPlayers []string `json:"mission_players"`
	AllowSuccess bool `json:"allow_success"`
	AllowFailure bool `json:"allow_failure"`
}

type GameStateOver struct {
	General GameState `json:"general"`
	Result string `json:"result"`
	Cards []string `json:"cards"`
}

func get_last_vote(c appengine.Context, game data.Game, pvotes *[]bool) error {
	if game.LastVoteMission == -1 || game.LastVoteProposal == -1 {
		return nil
	}

	votedata, err := db.GetVotes(c, game, game.LastVoteMission, game.LastVoteProposal)
	if err != nil {
		return err
	}

	votes := make([]bool, len(game.Players))
	for i, v := range(votedata) {
		votes[i] = v
	}
	*pvotes = votes

	return nil
}

func ReqGameState(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	myrole := game.Roles[mypos]

	proposal, err := db.GetProposal(c, game, game.ThisMission, game.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error retrieving proposal", 500}
	}

	results, err := db.GetMissionResults(c, game)
	if err != nil {
		return &web.AppError{err, "Error retrieving mission results", 500}
	}

	var votes []bool
	err = get_last_vote(c, game, &votes)

	general := GameState{
		Id: game.Id,
		Players: game.Players,
		State: "",
		Leader: game.Leader,
		Results: results,
		LastVotes: votes,
		ThisMission: game.ThisMission + 1,
		ThisProposal: game.ThisProposal + 1,
	}

	var state interface{}

	if game.GameOver {
		var result string
		if game.GoodScore >= 3 {
			result = "Good has won"
		} else {
			result = "Evil has won"
		}

		cards := make([]string, len(game.Players))
		for i := range game.Players {
			role := game.Roles[i]
			cards[i] = game.Setup.Cards[role].Label
		}

		general.State = "gameover"
		state = GameStateOver{
			General: general,
			Result: result,
			Cards: cards,
		}
	} else if proposal == nil {
		if err != nil {
			return &web.AppError{err, "Error retrieving votes", 500}
		}

		general.State = "picking"
		state = GameStatePicking{
			General: general,
			MissionSize: game.Setup.Missions[game.ThisMission].Size,
			MissionFailsAllowed: game.Setup.Missions[game.ThisMission].FailsAllowed,
		}
	} else {
		mission, err := db.GetMission(c, game, game.ThisMission)
		if err != nil {
			return &web.AppError{err, "Error retrieving mission", 500}
		}

		missionplayers := make([]string, len(proposal.Players))
		for i, n := range proposal.Players {
			missionplayers[i] = game.Players[n]
		}

		if mission == nil {
			general.State = "voting"
			state = GameStateVoting{
				General: general,
				MissionPlayers: missionplayers,
			}
		} else {
			general.State = "mission"

			state = GameStateMission{
				General: general,
				MissionPlayers: missionplayers,
				AllowSuccess: true,
				AllowFailure: game.Setup.Cards[myrole].Spy,
			}
		}
	}

	w.Header().Set("Content-type", "application/json")
	err = json.NewEncoder(w).Encode(&state)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}