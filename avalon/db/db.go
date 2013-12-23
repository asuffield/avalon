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

type GameFactory func(string, string) data.Game

func FindOrCreateGame(c appengine.Context, hangout string, pgame *data.Game, factory GameFactory) error {
	hangoutKey := datastore.NewKey(c, "Hangout", hangout, 0, nil)
	q := datastore.NewQuery("Game").Ancestor(hangoutKey).Filter("GameOver =", false).Order("-StartTime").Limit(1)
	var games []data.Game
	_, err := q.GetAll(c, &games)
	if err != nil {
		return err
	}
	if len(games) >= 1 {
		*pgame = games[0]
		return nil
	}

	var gameid string
	for {
		gameid = data.RandomString(64)
		oldgame, err := RetrieveGame(c, hangout, gameid)
		if err != nil {
			return err
		}
		if oldgame == nil {
			break
		}
	}

	game := factory(gameid, hangout)

	err = StoreGame(c, game)
	if err != nil {
		return err
	}

	*pgame = game
	return nil
}

func StoreGame(c appengine.Context, game data.Game) error {
	gameKey := makeGameKey(c, game)
	_, err := datastore.Put(c, gameKey, &game)
	return err
}

func RetrieveGame(c appengine.Context, hangoutid string, gameid string) (*data.Game, error) {
	var game data.Game
	hangoutKey := datastore.NewKey(c, "Hangout", hangoutid, 0, nil)
	gameKey := datastore.NewKey(c, "Game", gameid, 0, hangoutKey)
	err := datastore.Get(c, gameKey, &game)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	return &game, err
}

func RecentGames(c appengine.Context, limit int) ([]data.Game, error) {
	q := datastore.NewQuery("Game").Order("-StartTime").Limit(limit)
	var games []data.Game
	_, err := q.GetAll(c, &games)
	return games, err
}

func StoreProposal(c appengine.Context, game data.Game, proposal data.Proposal) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + game.ThisMission), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + game.ThisProposal), missionKey)
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

func StoreVote(c appengine.Context, game data.Game, pos int, approve bool) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + game.ThisMission), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + game.ThisProposal), missionKey)
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
func StoreMission(c appengine.Context, game data.Game) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + game.ThisMission), gameKey)
	mission := IntStore{game.ThisProposal}
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

func StoreMissionResult(c appengine.Context, game data.Game, result data.MissionResult) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "MissionResult", "", int64(1000 + game.ThisMission), gameKey)
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

func StoreAction(c appengine.Context, game data.Game, pos int, success bool) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + game.ThisMission), gameKey)
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
