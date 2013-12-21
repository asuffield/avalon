package avalon

import (
	"appengine"
	"avalon/auth"
	. "avalon/data"
	"avalon/db"
	"avalon/web"
	"encoding/json"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
	"strconv"
	"time"
	mathrand "math/rand"
    "log"
)

func init() {
	mathrand.Seed( time.Now().UTC().UnixNano())
	http.Handle("/app.js", web.AppHandler(auth.AppJS))
	http.Handle("/auth/token", web.AjaxHandler(auth.AuthToken))
	http.Handle("/game/start", web.AjaxHandler(game_start))
	http.Handle("/game/reveal", web.GameHandler(game_reveal))
	http.Handle("/game/propose", web.GameHandler(game_propose))
	http.Handle("/game/vote", web.GameHandler(game_vote))
	http.Handle("/game/mission", web.GameHandler(game_mission))
	http.Handle("/game/state", web.GameHandler(game_state))
}

type GameStartData struct {
	Participants map[string]string `json:"players"`
}

func shuffle_players(participants map[string]string) []string {
	players := make([]string, len(participants))
	order := mathrand.Perm(len(participants))
	i := 0
	for id := range participants {
		players[order[i]] = id
		i++
	}
	return players
}

func game_start(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	hangoutID, _ := session.Values["hangoutID"].(string)
	participantID, _ := session.Values["participantID"].(string)

	var gamestartdata GameStartData
	err := json.NewDecoder(r.Body).Decode(&gamestartdata)
	if err != nil {
		return &web.AppError{err, "Error storing parsing json body", 500}
	}

	if len(gamestartdata.Participants) > 5 {
		m := "Cannot have more than five players"
		return &web.AppError{errors.New(m), m, 500}
	}

	gameid := auth.RandomString(64)

	c := appengine.NewContext(r)
	oldgame, err := db.RetrieveGame(c, gameid)
	if err != nil {
		return &web.AppError{err, "Error fetching game", 500}
	} else if oldgame != nil {
		m := "Duplicate gameid created"
		return &web.AppError{errors.New(m), m, 500}
	}

	participants := map[string]string {}
	for k, v := range gamestartdata.Participants {
		participants[k] = v
	}

	// Fake it for testing purposes
	for i := len(participants); i < 5; i++ {
		participants[strconv.Itoa(i)] = "fake"
	}
	players := shuffle_players(participants)
	ais := make([]int, 0)
	for i, id := range players {
		if participants[id] == "fake" {
			ais = append(ais, i)
		}
	}

	game := Game{
		Id: gameid,
		Hangout: hangoutID,
		StartTime: time.Now(),
		Players: players,
		AIs: ais,
		Setup: MakeGameSetup(len(players)),
		Roles: mathrand.Perm(len(players)),
		Leader: 0,
		ThisMission: 0,
		ThisProposal: 0,
		LastVoteMission: -1,
		LastVoteProposal: -1,
		GoodScore: 0,
		EvilScore: 0,
		GameOver: false,
	}

	aerr := start_picking(c, game)
	if aerr != nil {
		return aerr
	}

	session.Values["gameID"] = gameid

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	playermap := MakePlayerMap(game.Players)
	mypos := playermap[participantID]

	return game_state(w, r, c, session, game, mypos)
}

type GameReveal struct {
	Players []string `json:"players"`
	Label string `json:"label"`
}

func game_reveal(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game Game, mypos int) *web.AppError {
	myrole := game.Roles[mypos]
	mycard := game.Setup.Cards[myrole]

	reveals := make([]GameReveal, 1)

	reveals[0] = GameReveal{Label: "Your card: " + game.Setup.Cards[myrole].Label, Players: []string{} }

	if mycard.Spy {
		players := make([]string, 0)
		for i := range game.Players {
			role := game.Roles[i]
			card := game.Setup.Cards[role]
			if card.Spy {
				players = append(players, game.Players[i])
			}
		}
		reveals = append(reveals, GameReveal{
			Label: "These are the evil players",
			Players: players,
		})
	}

	w.Header().Set("Content-type", "application/json")
	err := json.NewEncoder(w).Encode(&reveals)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}

type GameState struct {
	Id string `json:"gameid"`
	Players []string `json:"players"`
	State string `json:"state"`
	Leader int `json:"leader"`
	Results []MissionResult `json:"mission_results"`
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

func get_last_vote(c appengine.Context, game Game, pvotes *[]bool) error {
	log.Printf("Game: %+v", game)
	if game.LastVoteMission == -1 || game.LastVoteProposal == -1 {
		return nil
	}

	votedata, err := db.GetVotes(c, game, game.LastVoteMission, game.LastVoteProposal)
	if err != nil {
		return err
	}

	log.Printf("votedata: %+v", votedata)

	votes := make([]bool, len(game.Players))
	for i, v := range(votedata) {
		votes[i] = v
	}
	*pvotes = votes

	log.Printf("votes: %+v", votes)

	return nil
}

func count_score(results []MissionResult) (int, int) {
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

func game_state(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game Game, mypos int) *web.AppError {
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

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	w.Header().Set("Content-type", "application/json")
	err = json.NewEncoder(w).Encode(&state)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}

type ProposeData struct {
	Players []string `json:"players"`
}

func ai_votes(c appengine.Context, game Game) *web.AppError {
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

func ai_actions(c appengine.Context, game Game) *web.AppError {
	mission, err := db.GetMission(c, game, game.ThisMission)
	if err != nil {
		return &web.AppError{err, "Error retrieving mission", 500}
	}

	proposal, err := db.GetProposal(c, game, game.ThisMission, *mission)
	if err != nil {
		return &web.AppError{err, "Error retrieving proposal", 500}
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

func ai_proposal(c appengine.Context, game Game) *web.AppError {
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
			proposal := Proposal{ Leader: game.Leader, Players: players }
			//log.Printf("AI %s proposing: %v", game.Players[i], proposal)
			aerr := do_proposal(c, game, proposal)
			if aerr != nil {
				return aerr
			}
		}
	}

	return nil
}

func start_mission(c appengine.Context, game Game) *web.AppError {
	err := db.StoreMission(c, game)
	if err != nil {
		return &web.AppError{err, "Error storing mission", 500}
	}

	err = db.StoreGame(c, game)
	if err != nil {
		return &web.AppError{err, "Error storing game", 500}
	}

	aerr := ai_actions(c, game)
	if aerr != nil {
		return aerr
	}

	return nil
}

func start_picking(c appengine.Context, game Game) *web.AppError {
	aerr := ai_proposal(c, game)
	if aerr != nil {
		return aerr
	}

	err := db.StoreGame(c, game)
	if err != nil {
		return &web.AppError{err, "Error storing game", 500}
	}

	return nil
}

func do_proposal(c appengine.Context, game Game, proposal Proposal) *web.AppError {
	db.StoreProposal(c, game, proposal)

	if game.ThisProposal == 4 {
		// No vote on the 5th proposal - proceed directly to the mission
		game.LastVoteMission = game.ThisMission
		game.LastVoteProposal = -1

		return start_mission(c, game)
	}

	return ai_votes(c, game)
}

func do_vote(c appengine.Context, game Game, i int, vote bool, pvotes *map[int]bool) *web.AppError {
	var votes map[int]bool
	if (pvotes == nil) {
		var err error
		votes, err = db.GetVotes(c, game, game.ThisMission, game.ThisProposal)
		if err != nil {
			return &web.AppError{err, "Error fetching votes", 500}
		}
	} else {
		votes = *pvotes
	}

	err := db.StoreVote(c, game, i, vote)
	if err != nil {
		return &web.AppError{err, "Error storing mission", 500}
	}
	votes[i] = vote

	//log.Printf("Votes so far: %+v", votes)
	//log.Printf("Vote count %d, needed %d", len(votes), len(game.Players))

	if len(votes) == len(game.Players) {
		// Count the number of approve/reject votes
		approves, rejects := count_bools(votes)

		log.Printf("Votes: %+v", votes)
		log.Printf("%d approves, %d rejects", approves, rejects)

		game.LastVoteMission = game.ThisMission
		game.LastVoteProposal = game.ThisProposal
		log.Printf("set game: %+v", game)

		if approves > rejects {
			// Start mission
			aerr := start_mission(c, game)
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

			aerr := start_picking(c, game)
			if aerr != nil {
				return aerr
			}
		}
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

func do_action(c appengine.Context, game Game, i int, action bool, proposal Proposal, pactions *map[int]bool) *web.AppError {
	var actions map[int]bool
	if (pactions == nil) {
		actions, _ = db.GetActions(c, game, game.ThisMission)
	} else {
		actions = *pactions
	}

	db.StoreAction(c, game, i, action)
	actions[i] = action

	//log.Printf("Actions so far: %+v", actions)

	if len(actions) == game.Setup.Missions[game.ThisMission].Size {
		_, fails := count_bools(actions)

		result := MissionResult{Players: proposal.Players, Fails: fails, FailsAllowed: game.Setup.Missions[game.ThisMission].FailsAllowed}
		err := db.StoreMissionResult(c, game, result)
		if err != nil {
			return &web.AppError{err, "Error storing mission result", 500}
		}

		results, err := db.GetMissionResults(c, game)
		if err != nil {
			return &web.AppError{err, "Error retrieving mission results", 500}
		}

		game.GoodScore, game.EvilScore = count_score(results)
		game.GameOver = (game.GoodScore >= 3) || (game.EvilScore >= 3)

		if game.GameOver {
			err = db.StoreGame(c, game)
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

		aerr := start_picking(c, game)
		if aerr != nil {
			return aerr
		}
	}

	return nil
}

func game_propose(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game Game, mypos int) *web.AppError {
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

	playermap := MakePlayerMap(game.Players)
	proposal := Proposal{ Leader: game.Leader, Players: make([]int, len(proposedata.Players)) }
	i := 0
	for _, id := range proposedata.Players {
		p, ok := playermap[id]
		if !ok {
			m := "Invalid user in proposal"
			return &web.AppError{errors.New(m), m, 400}
		}
		proposal.Players[i] = p
		i++
	}

	do_proposal(c, game, proposal)

	pgame, err := db.RetrieveGame(c, game.Id)
	if err != nil {
		return &web.AppError{err, "Error refetching game", 500}
	}
	return game_state(w, r, c, session, *pgame, mypos)
}

type VoteData struct {
	Vote string `json:"vote"`
}

func game_vote(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game Game, mypos int) *web.AppError {
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
	aerr := do_vote(c, game, mypos, vote, &votes)
	if aerr != nil {
		return aerr
	}

	pgame, err := db.RetrieveGame(c, game.Id)
	if err != nil {
		return &web.AppError{err, "Error refetching game", 500}
	}
	return game_state(w, r, c, session, *pgame, mypos)
}

type ActionData struct {
	Action string `json:"action"`
}

func game_mission(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game Game, mypos int) *web.AppError {
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

	aerr := do_action(c, game, mypos, action, *proposal, &actions)
	if aerr != nil {
		return aerr
	}

	pgame, err := db.RetrieveGame(c, game.Id)
	if err != nil {
		return &web.AppError{err, "Error refetching game", 500}
	}
	return game_state(w, r, c, session, *pgame, mypos)
}
