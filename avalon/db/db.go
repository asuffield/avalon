package db

import (
	"appengine"
	"appengine/datastore"
	"avalon/data"
)

func makeGameKey(c appengine.Context, game data.Game) *datastore.Key {
	hangoutKey := datastore.NewKey(c, "Hangout", game.Hangout, 0, nil)
	gameKey := datastore.NewKey(c, "Game", game.Id, 0, hangoutKey)
	return gameKey
}

func makeGameStateKey(c appengine.Context, game data.Game) *datastore.Key {
	hangoutKey := datastore.NewKey(c, "Hangout", game.Hangout, 0, nil)
	gameKey := datastore.NewKey(c, "GameState", game.Id, 0, hangoutKey)
	return gameKey
}

type GameFactory func(string, string) data.Game

// Call with factory == nil to find and never create. With factory !=
// nil this function always returns a game or an error
func FindOrCreateGame(c appengine.Context, hangout string, factory GameFactory) (*data.Game, error) {
	hangoutKey := datastore.NewKey(c, "Hangout", hangout, 0, nil)
	q := datastore.NewQuery("Game").Ancestor(hangoutKey).Filter("GameOver =", false).Order("-StartTime").Limit(1)
	var games []data.Game
	_, err := q.GetAll(c, &games)
	if err != nil {
		return nil, err
	}
	if len(games) >= 1 {
		return &games[0], nil
	}

	if factory == nil {
		return nil, nil
	}

	var gameid string
	for {
		gameid = data.RandomString(64)
		oldgame, err := RetrieveGameStatic(c, hangout, gameid)
		if err != nil {
			return nil, err
		}
		if oldgame == nil {
			break
		}
	}

	game := factory(gameid, hangout)

	err = privStoreGame(c, game)
	if err != nil {
		return nil, err
	}

	err = StoreGameState(c, game)
	if err != nil {
		return nil, err
	}

	return &game, nil
}

func privStoreGame(c appengine.Context, game data.Game) error {
	gameKey := makeGameKey(c, game)
	_, err := datastore.Put(c, gameKey, &game.GameStatic)
	return err
}

func RetrieveGameStatic(c appengine.Context, hangoutid string, gameid string) (*data.GameStatic, error) {
	var game data.GameStatic
	hangoutKey := datastore.NewKey(c, "Hangout", hangoutid, 0, nil)
	gameKey := datastore.NewKey(c, "Game", gameid, 0, hangoutKey)
	err := datastore.Get(c, gameKey, &game)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	return &game, err
}

func StoreGameState(c appengine.Context, game data.Game) error {
	gameStateKey := makeGameStateKey(c, game)
	_, err := datastore.Put(c, gameStateKey, game.State)
	return err
}

func EnsureGameState(c appengine.Context, game *data.Game) error {
	if game.State != nil {
		return nil
	}
	gameStateKey := makeGameStateKey(c, *game)
	var state data.GameState
	err := datastore.Get(c, gameStateKey, &state)
	if err == nil {
		game.State = &state
	}
	return err
}

func RetrieveGame(c appengine.Context, hangoutid string, gameid string) (*data.Game, error) {
	gamestatic, err := RetrieveGameStatic(c, hangoutid, gameid)
	if err != nil {
		return nil, err
	}
	if gamestatic == nil {
		return nil, nil
	}
	game := data.Game{GameStatic: *gamestatic, State: nil}
	return &game, nil
}

func RecentGames(c appengine.Context, limit int) ([]data.Game, error) {
	q := datastore.NewQuery("Game").Order("-StartTime").Limit(limit)
	var gamestatics []data.GameStatic
	_, err := q.GetAll(c, &gamestatics)
	games := make([]data.Game, len(gamestatics))
	for i := range gamestatics {
		games[i].GameStatic = gamestatics[i]
	}
	return games, err
}

func StoreProposal(c appengine.Context, game data.Game, m int, p int, proposal data.Proposal) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	_, err := datastore.Put(c, proposalKey, &proposal)
	return err
}

func GetProposal(c appengine.Context, game data.Game, m int, p int) (*data.Proposal, error) {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	var proposal data.Proposal
	err := datastore.Get(c, proposalKey, &proposal)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	return &proposal, err
}

type BoolStore struct {
	Value bool
}

func StoreVote(c appengine.Context, game data.Game, m int, p int, pos int, approve bool) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	voteKey := datastore.NewKey(c, "Vote", "", int64(1000 + pos), proposalKey)
	vote := BoolStore{ approve }
	_, err := datastore.Put(c, voteKey, &vote)
	return err
}

func GetVotes(c appengine.Context, game data.Game, m int, p int) (map[int]bool, error) {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	q := datastore.NewQuery("Vote").Ancestor(proposalKey)
	var votes []BoolStore
	keys, err := q.GetAll(c, &votes)
	votemap := map[int]bool {}
	for i, k := range(keys) {
		votemap[int(k.IntID() - 1000)] = votes[i].Value
	}
	return votemap, err
}

type IntStore struct {
	Value int
}

// Missions just store the index of the proposal
func StoreMission(c appengine.Context, game data.Game, m int, p int) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000), gameKey)
	mission := IntStore{p}
	_, err := datastore.Put(c, missionKey, &mission)
	return err
}

func GetMission(c appengine.Context, game data.Game, m int) (*int, error) {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	var mission IntStore
	err := datastore.Get(c, missionKey, &mission)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	return &mission.Value, err
}

func StoreMissionResult(c appengine.Context, game data.Game, m int, result data.MissionResult) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "MissionResult", "", int64(1000 + m), gameKey)
	_, err := datastore.Put(c, missionKey, &result)
	return err
}

func GetMissionResults(c appengine.Context, game data.Game) ([]data.MissionResult, error) {
	gameKey := makeGameKey(c, game)
	q := datastore.NewQuery("MissionResult").Ancestor(gameKey)
	var results []data.MissionResult
	keys, err := q.GetAll(c, &results)
	resultsordered := make([]data.MissionResult, len(results))
	for i, k := range(keys) {
		resultsordered[int(k.IntID() - 1000)] = results[i]
	}
	return resultsordered, err
}

func StoreAction(c appengine.Context, game data.Game, m int, pos int, success bool) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	actionKey := datastore.NewKey(c, "Action", "", int64(1000 + pos), missionKey)
	action := BoolStore{ success }
	_, err := datastore.Put(c, actionKey, &action)
	return err
}

func GetActions(c appengine.Context, game data.Game, m int) (map[int]bool, error) {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	q := datastore.NewQuery("Action").Ancestor(missionKey)
	var actions []BoolStore
	keys, err := q.GetAll(c, &actions)
	actionmap := map[int]bool {}
	for i, k := range(keys) {
		actionmap[int(k.IntID() - 1000)] = actions[i].Value
	}
	return actionmap, err
}
