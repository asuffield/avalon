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

type GameStateGeneral struct {
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
	General GameStateGeneral `json:"general"`
	MissionSize int `json:"mission_size"`
	MissionFailsAllowed int `json:"mission_fails_allowed"`
}

type GameStateVoting struct {
	General GameStateGeneral `json:"general"`
	MissionPlayers []int `json:"mission_players"`
}

type GameStateMission struct {
	General GameStateGeneral `json:"general"`
	MissionPlayers []int `json:"mission_players"`
	AllowSuccess bool `json:"allow_success"`
	AllowFailure bool `json:"allow_failure"`
}

type GameStateOver struct {
	General GameStateGeneral `json:"general"`
	Result string `json:"result"`
	Cards []string `json:"cards"`
}

func get_last_vote(c appengine.Context, game data.Game, gamestate data.GameState, pvotes *[]bool) error {
	if gamestate.LastVoteMission == -1 || gamestate.LastVoteProposal == -1 {
		return nil
	}

	votedata, err := db.GetVotes(c, game, gamestate.LastVoteMission, gamestate.LastVoteProposal)
	if err != nil {
		return err
	}

	votes := make([]bool, len(gamestate.PlayerIDs))
	for i, v := range(votedata) {
		votes[i] = v
	}
	*pvotes = votes

	return nil
}

func MakeGameState(game data.Game, results []data.MissionResult, proposal *data.Proposal, mission *int, votes []bool, mypos int) interface{} {
	general := GameStateGeneral{
		Id: game.Id,
		Players: game.State.PlayerIDs,
		State: "",
		Leader: game.State.Leader,
		Results: results,
		LastVotes: votes,
		ThisMission: game.State.ThisMission + 1,
		ThisProposal: game.State.ThisProposal + 1,
	}

	if game.State.GameOver {
		var result string
		if game.State.GoodScore >= 3 {
			result = "Good has won"
		} else {
			result = "Evil has won"
		}

		cards := make([]string, len(game.Roles))
		for i, role := range game.Roles {
			cards[i] = game.Setup.Cards[role].Label
		}

		general.State = "gameover"
		return GameStateOver{
			General: general,
			Result: result,
			Cards: cards,
		}
	}

	if proposal == nil {
		general.State = "picking"
		return GameStatePicking{
			General: general,
			MissionSize: game.Setup.Missions[game.State.ThisMission].Size,
			MissionFailsAllowed: game.Setup.Missions[game.State.ThisMission].FailsAllowed,
		}
	}

	missionplayers := make([]int, len(proposal.Players))
	for i, n := range proposal.Players {
		missionplayers[i] = n
	}

	if mission == nil {
		general.State = "voting"
		return GameStateVoting{
			General: general,
			MissionPlayers: missionplayers,
		}
	}

	general.State = "mission"

	myrole := game.Roles[mypos]
	return GameStateMission{
		General: general,
		MissionPlayers: missionplayers,
		AllowSuccess: true,
		AllowFailure: game.Setup.Cards[myrole].Spy,
	}
}

func ReqGameState(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	err := db.EnsureGameState(c, &game)
	if err != nil {
		return &web.AppError{err, "Error retrieving game state", 500}
	}

	results, err := db.GetMissionResults(c, game)
	if err != nil {
		return &web.AppError{err, "Error retrieving mission results", 500}
	}

	var proposal *data.Proposal
	if !game.State.GameOver {
		proposal, err = db.GetProposal(c, game, game.State.ThisMission, game.State.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error retrieving proposal", 500}
		}
	}

	var mission *int
	if proposal != nil {
		mission, err = db.GetMission(c, game, game.State.ThisMission)
		if err != nil {
			return &web.AppError{err, "Error retrieving mission", 500}
		}
	}

	var votes []bool
	err = get_last_vote(c, game, *game.State, &votes)
	if err != nil {
		return &web.AppError{err, "Error retrieving last vote", 500}
	}

	state := MakeGameState(game, results, proposal, mission, votes, mypos)

	w.Header().Set("Content-type", "application/json")
	err = json.NewEncoder(w).Encode(&state)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}
