package start

import (
	"appengine"
	"appengine/datastore"
	"avalon/data"
	"avalon/db"
	"avalon/gameplay"
	"avalon/web"
	"encoding/json"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
	"strconv"
	"strings"
	"time"
	mathrand "math/rand"
    "log"
)

func init() {
	http.Handle("/game/start", web.AjaxHandler(ReqGameStart))
	http.Handle("/game/join", web.AjaxHandler(ReqGameJoin))
	http.Handle("/game/reveal", web.GameHandler(ReqGameReveal))
}

type GameStartData struct {
	Participants map[string]string `json:"players"`
}

type PlayerData struct {
	UserID string
	Name string
}

func shuffle_players(player_data []PlayerData) ([]string, []string) {
	players := make([]string, len(player_data))
	ordered_participants := make([]string, len(player_data))
	order := mathrand.Perm(len(player_data))
	i := 0
	for _, data := range player_data {
		players[order[i]] = data.Name
		ordered_participants[order[i]] = data.UserID
		i++
	}
	return players, ordered_participants
}

func game_factory(player_data []PlayerData) db.GameFactory {
	return func(gameid string, hangoutid string) data.Game {
		players, ordered_participants := shuffle_players(player_data)
		ais := make([]int, 0)
		for i, id := range players {
			if strings.HasPrefix(id, "ai_") {
				ais = append(ais, i)
			}
		}

		game := data.Game{
			Id: gameid,
			Hangout: hangoutid,
			StartTime: time.Now(),
			Participants: ordered_participants,
			Players: players,
			AIs: ais,
			Setup: data.MakeGameSetup(len(players)),
			Roles: mathrand.Perm(len(players)),
			Leader: -1, // See comment in ReqGameStart - this is the "start of game" marker
			ThisMission: 0,
			ThisProposal: 0,
			LastVoteMission: -1,
			LastVoteProposal: -1,
			GoodScore: 0,
			EvilScore: 0,
			GameOver: false,
		}

		return game
	}
}

func ReqGameStart(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
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

	player_data := make([]PlayerData, 0)
	for k, v := range gamestartdata.Participants {
		player_data = append(player_data, PlayerData{UserID: v, Name: k})
	}

	ai_count := 0
	if len(player_data) < 5 {
		ai_count = 5 - len(player_data)
	}

	// Fake it for testing purposes
	for i := 0; i < ai_count; i++ {
		player_data = append(player_data, PlayerData{UserID: "ai", Name: "ai_" + strconv.Itoa(i + 1)})
	}

	c := appengine.NewContext(r)
	var game data.Game
	err = datastore.RunInTransaction(c, func(tc appengine.Context) error {
		return db.FindOrCreateGame(tc, hangoutID, &game, game_factory(player_data))
	}, nil)
	if err != nil {
		return &web.AppError{err, "Error making game", 500}
	}

	// This step is critical: here we validate that the authenticated
	// userID is a participant in the game, before we hand them a
	// cryptographic cookie with the game in it
	mypos, ok := game.LookupUserID(userID)
	if !ok {
		m := "Not a user in the current game"
		return &web.AppError{errors.New(m), m, 500}
	}

	// Our participantID might have changed since the game started (if
	// we left and rejoined) so update it here
	if game.Players[mypos] != participantID {
		game.Players[mypos] = participantID
		db.StoreGame(c, game)
	}

	// We use leader == -1 as a "start of game" indicator, to make
	// sure we go through this step exactly once
	if game.Leader == -1 {
		game.Leader = 0

		var aerr *web.AppError
		err := datastore.RunInTransaction(c, func(tc appengine.Context) error {
			aerr = gameplay.StartPicking(tc, &game)
			return nil
		}, nil)
		if err != nil {
			return &web.AppError{err, "Error starting game (transaction failed?)", 500}
		}
		if aerr != nil {
			return aerr
		}
	}

	c.Infof("Joining game: %+v", game)

	session.Values["gameID"] = game.Id

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return gameplay.ReqGameState(w, r, c, session, game, mypos)
}

func ReqGameJoin(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	hangoutID, _ := session.Values["hangoutID"].(string)
	participantID, _ := session.Values["participantID"].(string)

	c := appengine.NewContext(r)
	pgame, err := db.FindGame(c, hangoutID)
	if err != nil {
		return &web.AppError{err, "Error finding game", 500}
	}
	if pgame == nil {
		m := "No game here to join"
		return &web.AppError{errors.New(m), m, 404}
	}

	game := *pgame

	// This step is critical: here we validate that the authenticated
	// userID is a participant in the game, before we hand them a
	// cryptographic cookie with the game in it
	mypos, ok := game.LookupUserID(userID)
	if !ok {
		m := "Not a user in the current game"
		return &web.AppError{errors.New(m), m, 500}
	}

	// Our participantID might have changed since the game started (if
	// we left and rejoined) so update it here
	if game.Players[mypos] != participantID {
		game.Players[mypos] = participantID
		db.StoreGame(c, game)
	}

	session.Values["gameID"] = game.Id

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return gameplay.ReqGameState(w, r, c, session, game, mypos)
}

type GameReveal struct {
	Players []string `json:"players"`
	Label string `json:"label"`
}

func ReqGameReveal(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
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
