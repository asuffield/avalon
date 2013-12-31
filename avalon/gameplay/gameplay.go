package gameplay

import (
	"appengine"
	"avalon/data"
	"avalon/db"
	"avalon/gameplay/state"
	"avalon/db/trans"
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

func count_score(results []*data.MissionResult) (int, int) {
	good := 0
	evil := 0
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.Fails > result.FailsAllowed {
			evil++
		} else {
			good++
		}
	}

	return good, evil
}

func ai_votes(c appengine.Context, game data.Game, proposal *data.Proposal) *web.AppError {
	for _, i := range game.AIs {
		vote := mathrand.Intn(2) == 1
		//log.Printf("AI %s vote: %v", game.Players[i], vote)
		aerr := do_vote(c, game, i, vote, proposal)
		if aerr != nil {
			return aerr
		}
	}

	return nil
}

func ai_actions(c appengine.Context, game data.Game, proposal data.Proposal, actions *data.Actions) *web.AppError {
	is_present := map[int]bool {}
	for _, i := range proposal.Players {
		is_present[i] = true
	}

	for _, i := range game.AIs {
		if is_present[i] {
			role := game.Roles[i]
			card := game.Cards[role]
			permitted := card.PermittedActions(game, proposal)
			action := !permitted["Failure"]
			//log.Printf("AI %s action: %v", game.Players[i], action)
			aerr := do_action(c, game, i, action, proposal, actions)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func ai_proposal(c appengine.Context, game data.Game) *web.AppError {
	for _, i := range game.AIs {
		if i == game.State.Leader {
			order := mathrand.Perm(len(game.Roles) - 1)
			size := game.Setup.Missions[game.State.ThisMission].Size
			players := make([]int, size)
			players[0] = i
			for j := 0; j < size - 1; j++ {
				pos := order[j]
				if pos >= i {
					pos++
				}
				players[j+1] = pos
			}
			//log.Printf("AI %s proposing: %v", game.Players[i], players)
			aerr := do_proposal(c, game, players)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func start_mission(c appengine.Context, game data.Game, proposal data.Proposal) *web.AppError {
	mission_size := game.Setup.Missions[game.State.ThisMission].Size
	actions := data.Actions{
		Mission: game.State.ThisMission,
		Proposal: game.State.ThisProposal,
		Actions: make([]bool, mission_size),
		Acted: make([]bool, mission_size),
	}
	err := db.StoreActions(c, game, game.State.ThisMission, actions)
	if err != nil {
		return &web.AppError{err, "Error storing actions object", 500}
	}

	game.State.HaveActions = true
	err = db.StoreGameState(c, game)
	if err != nil {
		return &web.AppError{err, "Error storing game state", 500}
	}

	aerr := ai_actions(c, game, proposal, &actions)
	if aerr != nil {
		return aerr
	}

	return nil
}

func StartPicking(c appengine.Context, game data.Game) *web.AppError {
	aerr := ai_proposal(c, game)
	if aerr != nil {
		return aerr
	}

	return nil
}

func do_proposal(c appengine.Context, game data.Game, players []int) *web.AppError {
	player_count := len(game.Roles)
	proposal := data.Proposal{ Leader: game.State.Leader, Players: players, Votes: make([]bool, player_count), Voted: make([]bool, player_count) }

	oldproposal, err := db.GetProposal(c, true, game, game.State.ThisMission, game.State.ThisProposal)
	if err != nil {
		return &web.AppError{err, "Error retrieving old proposal", 500}
	} else if oldproposal != nil {
		m := "Proposal has already been made"
		return &web.AppError{errors.New(m), m, 400}
	}

	if game.State.ThisProposal == 4 {
		// We represent the 5th proposal as having been unanimously approved
		for i := range proposal.Votes {
			proposal.Votes[i] = true
			proposal.Votes[i] = true
		}
	}

	db.StoreProposal(c, game, game.State.ThisMission, game.State.ThisProposal, proposal)

	game.State.HaveProposal = true
	err = db.StoreGameState(c, game)
	if err != nil {
		return &web.AppError{err, "Error storing game state", 500}
	}

	if game.State.ThisProposal == 4 {
		// No vote on the 5th proposal - proceed directly to the mission
		return start_mission(c, game, proposal)
	}

	return ai_votes(c, game, &proposal)
}

func check_votes(c appengine.Context, game data.Game, proposal data.Proposal) *web.AppError {
	//c.Debugf("Votes so far: %+v", votes)
	//c.Debugf("Vote count %d, needed %d", len(votes), len(game.Roles))

	_, unvoted := count_bools(proposal.Voted)

	if unvoted == 0 {
		voteresult := data.VoteResult {
			Index: game.State.ThisVote,
			Mission: game.State.ThisMission,
			Proposal: game.State.ThisProposal,
			Leader: proposal.Leader,
			Players: proposal.Players,
			Votes: proposal.Votes,
		}

		err := db.StoreVoteResult(c, game, voteresult)
		if err != nil {
			return &web.AppError{err, "Error storing vote result", 500}
		}

		// Count the number of approve/reject votes
		approves, rejects := count_bools(proposal.Votes)

		//c.Debugf("Votes: %+v", votes)
		//c.Debugf("%d approves, %d rejects", approves, rejects)

		game.State.ThisVote++
		err = db.StoreGameState(c, game)
		if err != nil {
			return &web.AppError{err, "Error storing game", 500}
		}

		if approves > rejects {
			// Start mission
			aerr := start_mission(c, game, proposal)
			if aerr != nil {
				return aerr
			}
		} else {
			// Move to next proposal
			game.State.ThisProposal++
			game.State.Leader++
			if game.State.Leader >= len(game.Roles) {
				game.State.Leader = 0
			}

			err := db.StoreGameState(c, game)
			if err != nil {
				return &web.AppError{err, "Error storing game", 500}
			}

			aerr := StartPicking(c, game)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func do_vote(c appengine.Context, game data.Game, i int, vote bool, proposal *data.Proposal) *web.AppError {
	proposal.Votes[i] = vote
	proposal.Voted[i] = true

	err := db.StoreProposal(c, game, game.State.ThisMission, game.State.ThisProposal, *proposal)
	if err != nil {
		return &web.AppError{err, "Error storing mission", 500}
	}

	aerr := check_votes(c, game, *proposal)
	if aerr != nil {
		return aerr
	}

	return nil
}

func count_bools(values []bool) (int, int) {
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

func check_actions(c appengine.Context, game data.Game, proposal data.Proposal, actions data.Actions) *web.AppError {
	//c.Debugf("Actions so far: %+v", actions)
	//c.Debugf("Action count %d, needed %d", len(actions), game.Setup.Missions[game.State.ThisMission].Size)

	_, unacted := count_bools(actions.Acted)

	if unacted == 0 {
		_, fails := count_bools(actions.Actions)

		// It would seem like we could call this after
		// StoreMissionResult - but this is misleading: we're in a
		// transaction and we will not see the newly added
		// MissionResult. So while we could call it there, placing it
		// up here makes it more clear what we're doing and also
		// avoids making the extra datastore read that will always
		// return nil
		results, err := db.GetMissionResults(c, game)
		if err != nil {
			return &web.AppError{err, "Error retrieving mission results", 500}
		}

		result := data.MissionResult{
			Mission: game.State.ThisMission,
			Proposal: game.State.ThisProposal,
			Leader: game.State.Leader,
			Players: proposal.Players,
			Fails: fails,
			FailsAllowed: game.Setup.Missions[game.State.ThisMission].FailsAllowed,
		}
		err = db.StoreMissionResult(c, game, game.State.ThisMission, result)
		if err != nil {
			return &web.AppError{err, "Error storing mission result", 500}
		}

		game.State.MissionsComplete[result.Mission] = true
		results = append(results, &result)

		game.State.GoodScore, game.State.EvilScore = count_score(results)
		game.State.GameOver = (game.State.GoodScore >= 3) || (game.State.EvilScore >= 3)

		if game.State.GameOver {
			err = db.StoreGameState(c, game)
			if err != nil {
				return &web.AppError{err, "Error storing game", 500}
			}
			return nil
		}

		game.State.Leader++
		if game.State.Leader >= len(game.Roles) {
			game.State.Leader = 0
		}
		game.State.ThisProposal = 0
		game.State.ThisMission++

		if game.State.ThisMission >= 5 {
			panic("Mission has gone past 5!")
		}

		game.State.HaveProposal = false
		game.State.HaveActions = false

		err = db.StoreGameState(c, game)
		if err != nil {
			return &web.AppError{err, "Error storing game", 500}
		}

		aerr := StartPicking(c, game)
		if aerr != nil {
			return aerr
		}
	}

	return nil
}

func do_action(c appengine.Context, game data.Game, mypos int, action bool, proposal data.Proposal, actions *data.Actions) *web.AppError {
	mpos, ok := proposal.LookupMissionSlot(mypos)
	if !ok {
		m := "Position is not on this mission"
		return &web.AppError{errors.New(m), m, 500}
	}

	actions.Actions[mpos] = action
	actions.Acted[mpos] = true

	err := db.StoreActions(c, game, game.State.ThisMission, *actions)
	if err != nil {
		return &web.AppError{err, "Error storing action", 500}
	}

	//log.Printf("Actions so far: %+v", actions)

	aerr := check_actions(c, game, proposal, *actions)
	if aerr != nil {
		return aerr
	}

	return nil
}

type ProposeData struct {
	Mission int `json:"mission"`
	Proposal int `json:"proposal"`
	Players []int `json:"players"`
}

func ValidateGamePropose(game data.Game, proposedata ProposeData, mypos int) *web.AppError {
	if game.State.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	if game.State.Leader != mypos {
		m := "You are not the leader"
		return &web.AppError{errors.New(m), m, 400}
	}

	if proposedata.Mission != game.State.ThisMission || proposedata.Proposal != game.State.ThisProposal {
		m := "Proposal is not current"
		return &web.AppError{errors.New(m), m, 400}
	}

	if len(proposedata.Players) != game.Setup.Missions[game.State.ThisMission].Size {
		m := "Sent wrong number of users"
		return &web.AppError{errors.New(m), m, 400}
	}

	for _, pos := range proposedata.Players {
		if pos < 0 || pos >= len(game.Roles) {
			m := "Invalid position in proposal"
			return &web.AppError{errors.New(m), m, 400}
		}
	}

	return nil
}

func ReqGamePropose(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	err := db.EnsureGameState(c, &game, true)
	if err != nil {
		return &web.AppError{err, "Error retrieving game state", 500}
	}

	var proposedata ProposeData
	err = json.NewDecoder(r.Body).Decode(&proposedata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	// These are 1-based in the ajax API
	proposedata.Mission--
	proposedata.Proposal--

	aerr := ValidateGamePropose(game, proposedata, mypos)
	if aerr != nil {
		return aerr
	}

	aerr = trans.RunGameTransaction(c, &game, func(tc appengine.Context, game data.Game) *web.AppError {
		return do_proposal(tc, game, proposedata.Players)
	})
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

type VoteData struct {
	Mission int `json:"mission"`
	Proposal int `json:"proposal"`
	Vote string `json:"vote"`
}

func ValidateGameVote(game data.Game, votedata VoteData) *web.AppError {
	if game.State.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	if game.State.ThisProposal >= 4 {
		m := "There is no vote on this mission"
		return &web.AppError{errors.New(m), m, 400}
	}

	if votedata.Mission != game.State.ThisMission || votedata.Proposal != game.State.ThisProposal {
		m := "Vote is not for the current proposal"
		return &web.AppError{errors.New(m), m, 400}
	}

	if votedata.Vote != "approve" && votedata.Vote != "reject" {
		m := "Invalid vote"
		return &web.AppError{errors.New(m), m, 400}
	}

	return nil
}

func ReqGameVote(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	err := db.EnsureGameState(c, &game, true)
	if err != nil {
		return &web.AppError{err, "Error retrieving game state", 500}
	}

	var votedata VoteData
	err = json.NewDecoder(r.Body).Decode(&votedata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	// These are 1-based in the ajax API
	votedata.Mission--
	votedata.Proposal--

	aerr := ValidateGameVote(game, votedata)
	if aerr != nil {
		return aerr
	}

	vote := votedata.Vote == "approve"

	aerr = trans.RunGameTransaction(c, &game, func(tc appengine.Context, game data.Game) *web.AppError {
		proposal, err := db.GetProposal(c, true, game, game.State.ThisMission, game.State.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error fetching proposal", 500}
		}
		if proposal == nil {
			m := "Proposal not found"
			return &web.AppError{errors.New(m), m, 500}
		}

		return do_vote(tc, game, mypos, vote, proposal)
	})
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

type ActionData struct {
	Mission int `json:"mission"`
	Proposal int `json:"proposal"`
	Action string `json:"action"`
}

func ValidateGameMission(game data.Game, actiondata ActionData, mypos int, proposal *data.Proposal) *web.AppError {
	if game.State.GameOver {
		m := "This game is over"
		return &web.AppError{errors.New(m), m, 400}
	}

	if proposal == nil {
		m := "No mission is in progress"
		return &web.AppError{errors.New(m), m, 400}
	}

	_, unvoted := count_bools(proposal.Voted)
	approved, rejected := count_bools(proposal.Votes)
	if unvoted != 0 || approved < rejected {
		m := "This proposal has not been approved"
		return &web.AppError{errors.New(m), m, 400}
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

	if actiondata.Mission != game.State.ThisMission || actiondata.Proposal != game.State.ThisProposal {
		m := "Action is not for the current proposal"
		return &web.AppError{errors.New(m), m, 400}
	}

	myrole := game.Roles[mypos]
	card := game.Cards[myrole]
	// Absent is the same error as known-but-forbidden
	permitted := card.PermittedActions(game, *proposal)[actiondata.Action]

	if !permitted {
		m := "Invalid action " + actiondata.Action
		return &web.AppError{errors.New(m), m, 400}
	}

	return nil
}

func ReqGameMission(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	err := db.EnsureGameState(c, &game, true)
	if err != nil {
		return &web.AppError{err, "Error retrieving game state", 500}
	}

	actions, err := db.GetActions(c, true, game, game.State.ThisMission)
	if err != nil {
		return &web.AppError{err, "Error retrieving actions", 500}
	}

	var proposal *data.Proposal
	if actions != nil {
		proposal, err = db.GetProposal(c, true, game, game.State.ThisMission, actions.Proposal)
		if err != nil {
			return &web.AppError{err, "Error retrieving proposal", 500}
		}
	}

	var actiondata ActionData
	err = json.NewDecoder(r.Body).Decode(&actiondata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	// These are 1-based in the ajax API
	actiondata.Mission--
	actiondata.Proposal--

	aerr := ValidateGameMission(game, actiondata, mypos, proposal)

	action := actiondata.Action == "success"

	aerr = trans.RunGameTransaction(c, &game, func(tc appengine.Context, game data.Game) *web.AppError {
		actions, err := db.GetActions(c, true, game, game.State.ThisMission)
		if err != nil {
			return &web.AppError{err, "Error retrieving actions", 500}
		}
		if actions == nil {
			return &web.AppError{err, "Actions object not found for current mission", 500}
		}

		return do_action(tc, game, mypos, action, *proposal, actions)
	})
	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}

func ReqGamePoke(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	aerr := trans.RunGameTransaction(c, &game, func(tc appengine.Context, game data.Game) *web.AppError {
		proposal, err := db.GetProposal(tc, true, game, game.State.ThisMission, game.State.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error retrieving proposal", 500}
		}

		actions, err := db.GetActions(tc, true, game, game.State.ThisMission)
		if err != nil {
			return &web.AppError{err, "Error fetching actions", 500}
		}

		aerr := check_votes(tc, game, *proposal)
		if aerr != nil {
			return aerr
		}

		aerr = check_actions(tc, game, *proposal, *actions)
		if aerr != nil {
			return aerr
		}

		return nil
	})

	if aerr != nil {
		return aerr
	}

	return state.ReqGameState(w, r, c, session, game, mypos)
}
