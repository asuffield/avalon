package db

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"avalon/data"
	"strconv"
	"strings"
	"time"
)

func makeCacheKey(keys ...string) string {
	return strings.Join(keys, "/")
}

func cacheGetObject(c appengine.Context, kind string, cacheKey string, v interface{}) bool {
	_, err := memcache.Gob.Get(c, cacheKey, v)
	if err == memcache.ErrCacheMiss {
		c.Debugf("%s cache miss at %s", kind, cacheKey)
		return false
	} else if err != nil  {
		c.Debugf("Error fetching %s from memcache: %s", kind, err)
		return false
	}
	return true
}

func cacheSetObject(c appengine.Context, kind string, cacheKey string, expiration int, v interface{}) {
	item := memcache.Item {
		Key: cacheKey,
		Object: v,
		Expiration: time.Duration(expiration) * time.Second,
	}
	err := memcache.Gob.Set(c, &item)
	if err != nil {
		c.Debugf("Error storing %s in memcache: %s", kind, err)
	}
	c.Debugf("Set %s at %s to %v", kind, cacheKey, v)
}

func cacheDeleteObject(c appengine.Context, kind string, cacheKey string) error {
	err := memcache.Delete(c, cacheKey)
	if err != nil && err != memcache.ErrCacheMiss {
		c.Debugf("Error deleting %s from memcache: %s", kind, err)
		return err
	}
	// Note that we suppress ErrCacheMiss here
	return nil
}

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

type GameFactory func(string, string) (data.Game, []string)

// Call with factory == nil to find and never create. With factory !=
// nil this function always returns a game or an error
func FindOrCreateGame(c appengine.Context, hangout string, factory GameFactory) (*data.Game, error) {
	hangoutKey := datastore.NewKey(c, "Hangout", hangout, 0, nil)
	// We will select the most recently stated game in this hangout
	q := datastore.NewQuery("Game").Ancestor(hangoutKey).Order("-StartTime").Limit(1)
	var games []data.GameStatic
	_, err := q.GetAll(c, &games)
	if err != nil {
		return nil, err
	}
	if len(games) >= 1 {
		game := data.Game{GameStatic: games[0], State: nil}
		err := EnsureGameState(c, &game, true)
		if err != nil {
			return nil, err
		}
		// If the most recently started game is over, we'll create a new game
		if !game.State.GameOver {
			return &game, nil
		}
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

	game, playerids := factory(gameid, hangout)

	err = privStoreGame(c, game)
	if err != nil {
		return nil, err
	}

	err = StoreGameState(c, game)
	if err != nil {
		return nil, err
	}

	for i, id := range playerids {
		err = StorePlayerID(c, game, i, id)
		if err != nil {
			return nil, err
		}
	}

	return &game, nil
}

func gameStaticCacheKey(hangoutid string, gameid string) string {
	return makeCacheKey("GameStatic", hangoutid, gameid)
}

func cacheGetGameStatic(c appengine.Context, hangoutid string, gameid string) *data.GameStatic {
	var static data.GameStatic
	ok := cacheGetObject(c, "GameStatic", gameStaticCacheKey(hangoutid, gameid), &static)
	if ok {
		return &static
	} else {
		return nil
	}
}

func cacheSetGameStatic(c appengine.Context, gamestatic data.GameStatic) {
	cacheSetObject(c, "GameStatic", gameStaticCacheKey(gamestatic.Hangout, gamestatic.Id), 600, gamestatic)
}

func privStoreGame(c appengine.Context, game data.Game) error {
	gameKey := makeGameKey(c, game)
	_, err := datastore.Put(c, gameKey, &game.GameStatic)
	return err
}

func RetrieveGameStatic(c appengine.Context, hangoutid string, gameid string) (*data.GameStatic, error) {
	pgame := cacheGetGameStatic(c, hangoutid, gameid)
	if pgame != nil {
		return pgame, nil
	}

	var game data.GameStatic
	hangoutKey := datastore.NewKey(c, "Hangout", hangoutid, 0, nil)
	gameKey := datastore.NewKey(c, "Game", gameid, 0, hangoutKey)
	err := datastore.Get(c, gameKey, &game)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	cacheSetGameStatic(c, game)
	return &game, err
}

func gameStateCacheKey(game data.Game) string {
	return makeCacheKey("GameState", game.Hangout, game.Id)
}

func cacheGetGameState(c appengine.Context, game data.Game) *data.GameState {
	var state data.GameState
	ok := cacheGetObject(c, "GameState", gameStateCacheKey(game), &state)
	if ok {
		return &state
	} else {
		return nil
	}
}

func cacheSetGameState(c appengine.Context, game data.Game) {
	// GameState is a little fragile - we rely on FlushGameStateCache
	// after a transaction to clear the cache. Since this might
	// possibly fail, we expire game state after 30 seconds so
	// game/state will eventually become consistent anyway
	cacheSetObject(c, "GameState", gameStateCacheKey(game), 30, *game.State)
}

func cacheDeleteGameState(c appengine.Context, game data.Game) error {
	return cacheDeleteObject(c, "GameState", gameStateCacheKey(game))
}

func StoreGameState(c appengine.Context, game data.Game) error {
	gameStateKey := makeGameStateKey(c, game)
	_, err := datastore.Put(c, gameStateKey, game.State)
	return err
}

func EnsureGameState(c appengine.Context, game *data.Game, uncached bool) error {
	if game.State != nil {
		return nil
	}
	if !uncached {
		pstate := cacheGetGameState(c, *game)
		if pstate != nil {
			game.State = pstate
			return nil
		}
	}

	gameStateKey := makeGameStateKey(c, *game)
	var state data.GameState
	err := datastore.Get(c, gameStateKey, &state)
	if err == nil {
		game.State = &state
		if !uncached {
			cacheSetGameState(c, *game)
		}
	}
	return err
}

func FlushGameStateCache(c appengine.Context, game data.Game) error {
	err := cacheDeleteProposal(c, game, game.State.ThisMission, game.State.ThisProposal)
	if err != nil {
		return err
	}
	err = cacheDeleteActions(c, game, game.State.ThisMission)
	if err != nil {
		return err
	}
	err = cacheDeleteGameState(c, game)
	if err != nil {
		return err
	}
	return nil
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

type StringStore struct {
	Value string
}

func playerIDCacheKey(game data.Game, pos int) string {
	return makeCacheKey("playerID", game.Hangout, game.Id, strconv.Itoa(pos))
}

func cacheGetPlayerID(c appengine.Context, game data.Game, pos int) string {
	var playerID StringStore
	ok := cacheGetObject(c, "PlayerID", playerIDCacheKey(game, pos), &playerID)
	if ok {
		return playerID.Value
	} else {
		return ""
	}
}

func cacheSetPlayerID(c appengine.Context, game data.Game, pos int, playerID string) {
	cacheSetObject(c, "PlayerID", playerIDCacheKey(game, pos), 600, StringStore {playerID})
}

func cacheDeletePlayerID(c appengine.Context, game data.Game, pos int) error {
	return cacheDeleteObject(c, "PlayerID", playerIDCacheKey(game, pos))
}

// This function must not be called from within a transaction - it
// touches memcache. Bad design, I know. Move it somewhere else.
func StorePlayerID(c appengine.Context, game data.Game, pos int, newid string) error {
	gameKey := makeGameKey(c, game)
	playerIDKey := datastore.NewKey(c, "PlayerID", "", int64(1000 + pos), gameKey)

	value := StringStore{ newid }
	_, err := datastore.Put(c, playerIDKey, &value)
	if err != nil {
		return err
	}

	// Datastore is now guaranteed to update

	// If we cannot invalidate the cache, we fail the operation (even
	// though datastore is updated), so the game/join request will
	// fail and be retried client-side
	err = cacheDeletePlayerID(c, game, pos)
	if err != nil {
		return err
	}

	// We're not in a transaction, so we may now safely prime the cache
	cacheSetPlayerID(c, game, pos, newid)
	return nil
}

func GetPlayerID(c appengine.Context, game data.Game, pos int) (string, error) {
	playerid := cacheGetPlayerID(c, game, pos)
	if playerid != "" {
		return playerid, nil
	}

	gameKey := makeGameKey(c, game)
	playerIDKey := datastore.NewKey(c, "PlayerID", "", int64(1000 + pos), gameKey)
	var value StringStore
	err := datastore.Get(c, playerIDKey, &value)
	if err == datastore.ErrNoSuchEntity {
		return "", nil
	}
	if err == nil {
		cacheSetPlayerID(c, game, pos, value.Value)
	}
	return value.Value, err
}

func GetPlayerIDs(c appengine.Context, game data.Game) ([]string, error) {
	playerids := make([]string, len(game.Roles))
	for i := range playerids {
		var err error
		playerids[i], err = GetPlayerID(c, game, i)
		if err != nil {
			return []string{}, err
		}
	}
	return playerids, nil
}

func proposalCacheKey(game data.Game, m int, p int) string {
	return makeCacheKey("proposal", game.Hangout, game.Id, strconv.Itoa(m), strconv.Itoa(p))
}

func cacheGetProposal(c appengine.Context, game data.Game, m int, p int) *data.Proposal {
	var proposal data.Proposal
	ok := cacheGetObject(c, "Proposal", proposalCacheKey(game, m, p), &proposal)
	if ok {
		return &proposal
	} else {
		return nil
	}
}

func cacheSetProposal(c appengine.Context, game data.Game, m int, p int, proposal data.Proposal) {
	// Proposal is a little fragile - we rely on FlushGameStateCache
	// after a transaction to clear the cache. Since this might
	// possibly fail, we expire proposals after 30 seconds so
	// game/state will eventually become consistent anyway
	cacheSetObject(c, "Proposal", proposalCacheKey(game, m, p), 30, proposal)
}

func cacheDeleteProposal(c appengine.Context, game data.Game, m int, p int) error {
	return cacheDeleteObject(c, "Proposal", proposalCacheKey(game, m, p))
}

func StoreProposal(c appengine.Context, game data.Game, m int, p int, proposal data.Proposal) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	_, err := datastore.Put(c, proposalKey, &proposal)
	return err
}

func GetProposal(c appengine.Context, uncached bool, game data.Game, m int, p int) (*data.Proposal, error) {
	if !uncached {
		pproposal := cacheGetProposal(c, game, m, p)
		if pproposal != nil {
			return pproposal, nil
		}
	}

	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "Mission", "", int64(1000 + m), gameKey)
	proposalKey := datastore.NewKey(c, "Proposal", "", int64(1000 + p), missionKey)
	var proposal data.Proposal
	err := datastore.Get(c, proposalKey, &proposal)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	if err == nil && !uncached {
		cacheSetProposal(c, game, m, p, proposal)
	}
	return &proposal, err
}

func actionsCacheKey(game data.Game, m int) string {
	return makeCacheKey("actions", game.Hangout, game.Id, strconv.Itoa(m))
}

func cacheGetActions(c appengine.Context, game data.Game, m int) *data.Actions {
	var actions data.Actions
	ok := cacheGetObject(c, "Actions", actionsCacheKey(game, m), &actions)
	if ok {
		return &actions
	} else {
		return nil
	}
}

func cacheSetActions(c appengine.Context, game data.Game, m int, actions data.Actions) {
	// Actions is a little fragile - we rely on FlushGameStateCache
	// after a transaction to clear the cache. Since this might
	// possibly fail, we expire actions after 30 seconds so
	// game/state will eventually become consistent anyway
	cacheSetObject(c, "Actions", actionsCacheKey(game, m), 30, actions)
}

func cacheDeleteActions(c appengine.Context, game data.Game, m int) error {
	return cacheDeleteObject(c, "Actions", actionsCacheKey(game, m))
}

func StoreActions(c appengine.Context, game data.Game, m int, actions data.Actions) error {
	gameKey := makeGameKey(c, game)
	actionsKey := datastore.NewKey(c, "Actions", "", int64(1000 + m), gameKey)
	_, err := datastore.Put(c, actionsKey, &actions)
	return err
}

func GetActions(c appengine.Context, uncached bool, game data.Game, m int) (*data.Actions, error) {
	if !uncached {
		pactions := cacheGetActions(c, game, m)
		if pactions != nil {
			return pactions, nil
		}
	}

	gameKey := makeGameKey(c, game)
	actionsKey := datastore.NewKey(c, "Actions", "", int64(1000 + m), gameKey)

	var actions data.Actions
	err := datastore.Get(c, actionsKey, &actions)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	if err == nil && !uncached {
		cacheSetActions(c, game, m, actions)
	}
	return &actions, err
}

func missionResultCacheKey(game data.Game, m int) string {
	return makeCacheKey("missionResult", game.Hangout, game.Id, strconv.Itoa(m))
}

func cacheGetMissionResult(c appengine.Context, game data.Game, m int) *data.MissionResult {
	var missionResult data.MissionResult
	ok := cacheGetObject(c, "MissionResult", missionResultCacheKey(game, m), &missionResult)
	if ok {
		return &missionResult
	} else {
		return nil
	}
}

func cacheSetMissionResult(c appengine.Context, game data.Game, m int, missionResult data.MissionResult) {
	cacheSetObject(c, "MissionResult", missionResultCacheKey(game, m), 600, missionResult)
}

func StoreMissionResult(c appengine.Context, game data.Game, m int, result data.MissionResult) error {
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "MissionResult", "", int64(1000 + m), gameKey)
	_, err := datastore.Put(c, missionKey, &result)
	return err
}

func GetMissionResult(c appengine.Context, game data.Game, m int) (*data.MissionResult, error) {
	presult := cacheGetMissionResult(c, game, m)
	if presult != nil {
		return presult, nil
	}
	gameKey := makeGameKey(c, game)
	missionKey := datastore.NewKey(c, "MissionResult", "", int64(1000 + m), gameKey)
	var result data.MissionResult
	err := datastore.Get(c, missionKey, &result)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	if err == nil {
		cacheSetMissionResult(c, game, m, result)
	}
	return &result, err
}

func GetMissionResults(c appengine.Context, game data.Game) ([]*data.MissionResult, error) {
	results := make([]*data.MissionResult, 0)
	for i, complete := range game.State.MissionsComplete {
		if complete {
			presult, err := GetMissionResult(c, game, i)
			if err != nil {
				return results, err
			}
			results = append(results, presult)
		}
	}

	return results, nil
}

func voteResultCacheKey(game data.Game, r int) string {
	return makeCacheKey("voteResult", game.Hangout, game.Id, strconv.Itoa(r))
}

func cacheGetVoteResult(c appengine.Context, game data.Game, r int) *data.VoteResult {
	var voteResult data.VoteResult
	ok := cacheGetObject(c, "VoteResult", voteResultCacheKey(game, r), &voteResult)
	if ok {
		return &voteResult
	} else {
		return nil
	}
}

func cacheSetVoteResult(c appengine.Context, game data.Game, r int, voteResult data.VoteResult) {
	cacheSetObject(c, "VoteResult", voteResultCacheKey(game, r), 600, voteResult)
}

func StoreVoteResult(c appengine.Context, game data.Game, result data.VoteResult) error {
	gameKey := makeGameKey(c, game)
	voteResultKey := datastore.NewKey(c, "VoteResult", "", int64(1000 + result.Index), gameKey)
	_, err := datastore.Put(c, voteResultKey, &result)
	return err
}

func GetVoteResult(c appengine.Context, game data.Game, r int) (*data.VoteResult, error) {
	presult := cacheGetVoteResult(c, game, r)
	if presult != nil {
		return presult, nil
	}
	gameKey := makeGameKey(c, game)
	voteResultKey := datastore.NewKey(c, "VoteResult", "", int64(1000 + r), gameKey)
	var result data.VoteResult
	err := datastore.Get(c, voteResultKey, &result)
	if err == datastore.ErrNoSuchEntity {
		c.Debugf("No such vote result %d", r)
		return nil, nil
	}
	if err == nil {
		cacheSetVoteResult(c, game, r, result)
	}
	return &result, err
}

func GetVoteResults(c appengine.Context, game data.Game) ([]data.VoteResult, error) {
	results := make([]data.VoteResult, game.State.ThisVote)
	for i := 0; i < game.State.ThisVote; i++ {
		presult, err := GetVoteResult(c, game, i)
		if err != nil {
			return results, err
		}
		results[i] = *presult
	}
	return results, nil
}
