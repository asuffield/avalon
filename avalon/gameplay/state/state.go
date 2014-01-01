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
	Setup data.GameSetup `json:"setup"`
	Players []string `json:"players"`
	State string `json:"state"`
	Leader int `json:"leader"`
	Results []*data.MissionResult `json:"mission_results"`
	Votes []data.VoteResult `json:"votes"`
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
	VotedPlayers []bool `json:"voted_players"`
}

type GameStateMission struct {
	General GameStateGeneral `json:"general"`
	MissionPlayers []int `json:"mission_players"`
	ActedPlayers []bool `json:"acted_players"`
	AllowActions map[string]bool `json:"allow_actions"`
}

type GameStateAssassination struct {
	General GameStateGeneral `json:"general"`
	Assassin int `json:"assassin"`
	Cards []string `json:"cards"`
}

type GameStateOver struct {
	General GameStateGeneral `json:"general"`
	AssassinTarget int `json:"assassin_target"`
	Result string `json:"result"`
	Comment string `json:"comment"`
	Cards []string `json:"cards"`
}

func MakeGameState(game data.Game, playerids []string, results []*data.MissionResult, proposal *data.Proposal, actions *data.Actions, votes []data.VoteResult, mypos int) interface{} {
	general := GameStateGeneral{
		Id: game.Id,
		Setup: game.Setup,
		Players: playerids,
		State: "",
		Leader: game.State.Leader,
		Results: results,
		Votes: votes,
		ThisMission: game.State.ThisMission + 1,
		ThisProposal: game.State.ThisProposal + 1,
	}

	if game.State.GameOver {
		var result string
		var comment string

		if game.State.AssassinTarget != -1 && game.Cards[game.Roles[game.State.AssassinTarget]].Label() == "Merlin" {
			result = "Merlin has been assassinated"
		} else if game.State.GoodScore >= 3 {
			result = "Good has won"
		} else {
			result = "Evil has won"
		}

		myrole := game.Roles[mypos]
		mycard := game.Cards[myrole]
		if mycard.HasWon(game) {
			comment = "Victory!"
		} else {
			comment = "Defeat!"
		}

		cards := make([]string, len(game.Roles))
		for i, role := range game.Roles {
			cards[i] = game.Cards[role].Label()
		}

		general.State = "gameover"
		return GameStateOver{
			General: general,
			AssassinTarget: game.State.AssassinTarget,
			Result: result,
			Comment: comment,
			Cards: cards,
		}
	}

	if game.State.GoodScore >= 3 {
		// Must be in the assassination phase
		assassin := game.FindAssassin()
		if assassin == -1 {
			panic("Should be in assassination phase, but we have no assassin!")
		}

		cards := make([]string, len(game.Roles))
		for i, role := range game.Roles {
			// We'll use the players who "haven't won" as the evil
			// players - those are the same thing for now
			if !game.Cards[role].HasWon(game) {
				cards[i] = game.Cards[role].Label()
			}
		}

		general.State = "assassination"
		return GameStateAssassination{
			General: general,
			Assassin: assassin,
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

	if actions == nil {
		general.State = "voting"
		return GameStateVoting{
			General: general,
			MissionPlayers: missionplayers,
			VotedPlayers: proposal.Voted,
		}
	}

	general.State = "mission"

	myrole := game.Roles[mypos]
	return GameStateMission{
		General: general,
		MissionPlayers: missionplayers,
		ActedPlayers: actions.Acted,
		AllowActions: game.Cards[myrole].PermittedActions(game, *proposal),
	}
}

func ReqGameState(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	err := db.EnsureGameState(c, &game, false)
	if err != nil {
		return &web.AppError{err, "Error retrieving game state", 500}
	}

	playerids, err := db.GetPlayerIDs(c, game)
	if err != nil {
		return &web.AppError{err, "Error retrieving player ids", 500}
	}

	results, err := db.GetMissionResults(c, game)
	if err != nil {
		return &web.AppError{err, "Error retrieving mission results", 500}
	}

	votes, err := db.GetVoteResults(c, game)
	if err != nil {
		return &web.AppError{err, "Error retrieving vote results", 500}
	}

	var proposal *data.Proposal
	var actions *data.Actions
	if !game.State.GameOver {
		if game.State.HaveProposal {
			// Note that this is the only place we use the memcached
			// proposal - it might be a little out of date if a memcache
			// store operation fails, and that's ok because the only
			// update we send is whether people have voted yet
			proposal, err = db.GetProposal(c, false, game, game.State.ThisMission, game.State.ThisProposal)
			if err != nil {
				return &web.AppError{err, "Error retrieving proposal", 500}
			}
		}

		if game.State.HaveActions {
			actions, err = db.GetActions(c, false, game, game.State.ThisMission)
			if err != nil {
				return &web.AppError{err, "Error retrieving actions", 500}
			}
		}
	}

	state := MakeGameState(game, playerids, results, proposal, actions, votes, mypos)

	w.Header().Set("Content-type", "application/json")
	err = json.NewEncoder(w).Encode(&state)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}
