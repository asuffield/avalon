package gameplay

import (
	"appengine"
	"appengine/datastore"
	"avalon/data"
	"avalon/db"
	"avalon/gameplay/state"
	"avalon/web"
	"encoding/json"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
	mathrand "math/rand"
)

func init() {
	http.Handle("/game/propose", web.GameHandler(ReqGamePropose))
	http.Handle("/game/vote", web.GameHandler(ReqGameVote))
	http.Handle("/game/mission", web.GameHandler(ReqGameMission))
	http.Handle("/game/poke", web.GameHandler(ReqGamePoke))
}

func count_score(results []data.MissionResult) (int, int) {
	good := 0
	evil := 0
	for _, result := range results {
		if result.Fails > result.FailsAllowed {
			evil++
		} else {
			good++
		}
	}

	return good, evil
}

func ai_votes(c appengine.Context, game *data.Game) *web.AppError {
	for _, i := range game.AIs {
		vote := mathrand.Intn(2) == 1
		//log.Printf("AI %s vote: %v", game.Players[i], vote)
		aerr := do_vote(c, game, i, vote, nil)
		if aerr != nil {
			return aerr
		}
	}

	return nil
}

func ai_actions(c appengine.Context, game *data.Game, proposal *data.Proposal) *web.AppError {
	// Note that there is only one call site to this function, and
	// ThisProposal is always the current one, so we don't need to
	// call db.GetMission - AI mode is just a hack anyway

	if proposal == nil {
		var err error
		proposal, err = db.GetProposal(c, *game, game.ThisMission, game.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error retrieving proposal", 500}
		}
	}

	is_present := map[int]bool {}
	for _, i := range proposal.Players {
		is_present[i] = true
	}

	for _, i := range game.AIs {
		if is_present[i] {
			role := game.Roles[i]
			card := game.Setup.Cards[role]
			action := !card.Spy
			//log.Printf("AI %s action: %v", game.Players[i], action)
			aerr := do_action(c, game, i, action, *proposal, nil)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func ai_proposal(c appengine.Context, game *data.Game) *web.AppError {
	for _, i := range game.AIs {
		if i == game.Leader {
			order := mathrand.Perm(len(game.Players) - 1)
			size := game.Setup.Missions[game.ThisMission].Size
			players := make([]int, size)
			players[0] = i
			for j := 0; j < size - 1; j++ {
				pos := order[j]
				if pos >= i {
					pos++
				}
				players[j+1] = pos
			}
			proposal := data.Proposal{ Leader: game.Leader, Players: players }
			//log.Printf("AI %s proposing: %v", game.Players[i], proposal)
			aerr := do_proposal(c, game, proposal)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func start_mission(c appengine.Context, game *data.Game, proposal *data.Proposal) *web.AppError {
	err := db.StoreMission(c, *game)
	if err != nil {
		return &web.AppError{err, "Error storing mission", 500}
	}

	err = db.StoreGame(c, *game)
	if err != nil {
		return &web.AppError{err, "Error storing game", 500}
	}

	aerr := ai_actions(c, game, proposal)
	if aerr != nil {
		return aerr
	}

	return nil
}

func StartPicking(c appengine.Context, game *data.Game) *web.AppError {
	aerr := ai_proposal(c, game)
	if aerr != nil {
		return aerr
	}

	err := db.StoreGame(c, *game)
	if err != nil {
		return &web.AppError{err, "Error storing game", 500}
	}

	return nil
}

func do_proposal(c appengine.Context, game *data.Game, proposal data.Proposal) *web.AppError {
	db.StoreProposal(c, *game, proposal)

	if game.ThisProposal == 4 {
		// No vote on the 5th proposal - proceed directly to the mission
		game.LastVoteMission = game.ThisMission
		game.LastVoteProposal = -1

		return start_mission(c, game, &proposal)
	}

	return ai_votes(c, game)
}

func check_votes(c appengine.Context, game *data.Game, votes map[int]bool) *web.AppError {
	//log.Printf("Votes so far: %+v", votes)
	//log.Printf("Vote count %d, needed %d", len(votes), len(game.Players))

	if len(votes) == len(game.Players) {
		// Count the number of approve/reject votes
		approves, rejects := count_bools(votes)

		//log.Printf("Votes: %+v", votes)
		//log.Printf("%d approves, %d rejects", approves, rejects)

		game.LastVoteMission = game.ThisMission
		game.LastVoteProposal = game.ThisProposal

		if approves > rejects {
			// Start mission
			aerr := start_mission(c, game, nil)
			if aerr != nil {
				return aerr
			}
		} else {
			// Move to next proposal
			game.ThisProposal++
			game.Leader++
			if game.Leader >= len(game.Players) {
				game.Leader = 0
			}

			aerr := StartPicking(c, game)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func do_vote(c appengine.Context, game *data.Game, i int, vote bool, pvotes *map[int]bool) *web.AppError {
	var votes map[int]bool
	if (pvotes == nil) {
		var err error
		votes, err = db.GetVotes(c, *game, game.ThisMission, game.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error fetching votes", 500}
		}
	} else {
		votes = *pvotes
	}

	err := db.StoreVote(c, *game, i, vote)
	if err != nil {
		return &web.AppError{err, "Error storing mission", 500}
	}
	votes[i] = vote

	aerr := check_votes(c, game, votes)
	if aerr != nil {
		return aerr
	}

	return nil
}

func count_bools(values map[int]bool) (int, int) {
	trues := 0
	falses := 0
	for _, val := range values {
		if val {
			trues++
		} else {
			falses++
		}
	}
	return trues, falses
}

func check_actions(c appengine.Context, game *data.Game, proposal data.Proposal, actions map[int]bool) *web.AppError {
	if len(actions) == game.Setup.Missions[game.ThisMission].Size {
		_, fails := count_bools(actions)

		result := data.MissionResult{Players: proposal.Players, Fails: fails, FailsAllowed: game.Setup.Missions[game.ThisMission].FailsAllowed}
		err := db.StoreMissionResult(c, *game, result)
		if err != nil {
			return &web.AppError{err, "Error storing mission result", 500}
		}

		results, err := db.GetMissionResults(c, *game)
		if err != nil {
			return &web.AppError{err, "Error retrieving mission results", 500}
		}

		game.GoodScore, game.EvilScore = count_score(results)
		game.GameOver = (game.GoodScore >= 3) || (game.EvilScore >= 3)

		if game.GameOver {
			err = db.StoreGame(c, *game)
			if err != nil {
				return &web.AppError{err, "Error storing game", 500}
			}
			return nil
		}

		game.Leader++
		if game.Leader >= len(game.Players) {
			game.Leader = 0
		}
		game.ThisProposal = 0
		game.ThisMission++

		if game.ThisMission >= 5 {
			panic("Mission has gone past 5!")
		}

		aerr := StartPicking(c, game)
		if aerr != nil {
			return aerr
		}
	}

	return nil
}

func do_action(c appengine.Context, game *data.Game, i int, action bool, proposal data.Proposal, pactions *map[int]bool) *web.AppError {
	var actions map[int]bool
	if (pactions == nil) {
		actions, _ = db.GetActions(c, *game, game.ThisMission)
	} else {
		actions = *pactions
	}

	db.StoreAction(c, *game, i, action)
	actions[i] = action

	//log.Printf("Actions so far: %+v", actions)

	aerr := check_actions(c, game, proposal, actions)
	if aerr != nil {
		return aerr
	}

	return nil
}

type ProposeData struct {
	Players []int `json:"players"`
}

func ReqGamePropose(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	if game.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	if game.Leader != mypos {
		m := "You are not the leader"
		return &web.AppError{errors.New(m), m, 400}
	}

	oldproposal, err := db.GetProposal(c, game, game.ThisMission, game.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error retrieving proposal", 500}
	} else if oldproposal != nil {
		m := "Proposal has already been made"
		return &web.AppError{errors.New(m), m, 400}
	}

	var proposedata ProposeData
	err = json.NewDecoder(r.Body).Decode(&proposedata)
	if err != nil {
		return &web.AppError{err, "Error storing parsing json body", 500}
	}

	if len(proposedata.Players) != game.Setup.Missions[game.ThisMission].Size {
		m := "Sent wrong number of users"
		return &web.AppError{errors.New(m), m, 400}
	}

	for _, pos := range proposedata.Players {
		if pos < 0 || pos >= len(game.Players) {
			m := "Invalid posotion in proposal"
			return &web.AppError{errors.New(m), m, 400}
		}
	}

	proposal := data.Proposal{ Leader: game.Leader, Players: proposedata.Players }

	var aerr *web.AppError
	err = datastore.RunInTransaction(c, func(tc appengine.Context) error {
		aerr = do_proposal(tc, &game, proposal)
		return nil
	}, nil)
	if err != nil {
		return &web.AppError{err, "Error applying proposal (transaction failed?)", 500}
	}
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

type VoteData struct {
	Vote string `json:"vote"`
}

func ReqGameVote(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	if game.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	if game.ThisProposal >= 4 {
		m := "There is no vote on this mission"
		return &web.AppError{errors.New(m), m, 400}
	}

	votes, err := db.GetVotes(c, game, game.ThisMission, game.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error fetching votes", 500}
	}

	_, ok := votes[mypos]
	if ok {
		m := "You have already voted"
		return &web.AppError{errors.New(m), m, 400}
	}

	var votedata VoteData
	err = json.NewDecoder(r.Body).Decode(&votedata)
	if err != nil {
		return &web.AppError{err, "Error storing parsing json body", 500}
	}

	if votedata.Vote != "approve" && votedata.Vote != "reject" {
		return &web.AppError{err, "Invalid vote", 400}
	}

	vote := votedata.Vote == "approve"

	var aerr *web.AppError
	err = datastore.RunInTransaction(c, func(tc appengine.Context) error {
		aerr = do_vote(c, &game, mypos, vote, &votes)
		return nil
	}, nil)
	if err != nil {
		return &web.AppError{err, "Error applying proposal (transaction failed?)", 500}
	}
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

type ActionData struct {
	Action string `json:"action"`
}

func ReqGameMission(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	if game.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	mission, err := db.GetMission(c, game, game.ThisMission)
	if err != nil {
		return &web.AppError{err, "Error retrieving mission", 500}
	}
	if mission == nil {
		m := "No mission is in progress"
		return &web.AppError{errors.New(m), m, 400}
	}

	proposal, err := db.GetProposal(c, game, game.ThisMission, *mission)
	if err != nil {
		return &web.AppError{err, "Error retrieving proposal", 500}
	}

	found := false
	for _, pos := range proposal.Players {
		if pos == mypos {
			found = true
		}
	}
	if !found {
		m := "You are not on this mission"
		return &web.AppError{errors.New(m), m, 400}
	}

	actions, err := db.GetActions(c, game, game.ThisMission)
	if err != nil {
		return &web.AppError{err, "Error fetching actions", 500}
	}

	_, ok := actions[mypos]
	if ok {
		m := "You have already chosen your action"
		return &web.AppError{errors.New(m), m, 400}
	}

	var actiondata ActionData
	err = json.NewDecoder(r.Body).Decode(&actiondata)
	if err != nil {
		return &web.AppError{err, "Error storing parsing json body", 500}
	}

	if actiondata.Action != "success" && actiondata.Action != "fail" {
		m := "Invalid action " + actiondata.Action
		return &web.AppError{errors.New(m), m, 400}
	}

	action := actiondata.Action == "success"

	myrole := game.Roles[mypos]
	mycard := game.Setup.Cards[myrole]

	if !action && !mycard.Spy {
		m := "Invalid action - must pick success"
		return &web.AppError{errors.New(m), m, 400}
	}

	var aerr *web.AppError
	err = datastore.RunInTransaction(c, func(tc appengine.Context) error {
		aerr = do_action(c, &game, mypos, action, *proposal, &actions)
		return nil
	}, nil)
	if err != nil {
		return &web.AppError{err, "Error applying proposal (transaction failed?)", 500}
	}
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

func ReqGamePoke(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	votes, err := db.GetVotes(c, game, game.ThisMission, game.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error fetching votes", 500}
	}

	proposal, err := db.GetProposal(c, game, game.ThisMission, game.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error retrieving proposal", 500}
	}

	actions, err := db.GetActions(c, game, game.ThisMission)
	if err != nil {
		return &web.AppError{err, "Error fetching actions", 500}
	}

	aerr := check_votes(c, &game, votes)
	if aerr != nil {
		return aerr
	}

	aerr = check_actions(c, &game, *proposal, actions)
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}
