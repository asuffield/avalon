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

func game_factory(gamestartdata GameStartData) db.GameFactory {
	return func(gameid string, hangoutid string) data.Game {
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

func DoGameStartOrJoin(c appengine.Context, session *sessions.Session, factory db.GameFactory) (*data.Game, int, *web.AppError) {
	var pgame *data.Game
	err := datastore.RunInTransaction(c, func(tc appengine.Context) error {
		hangoutID, _ := session.Values["hangoutID"].(string)
		var dberr error
		pgame, dberr = db.FindOrCreateGame(tc, hangoutID, factory)
		return dberr
	}, nil)
	if err != nil {
		return nil, -1, &web.AppError{err, "Error making game", 500}
	}
	if pgame == nil {
		// We can only get here if factory is nil
		m := "No game here to join"
		return nil, -1, &web.AppError{errors.New(m), m, 404}
	}

	// We use leader == -1 as a "start of game" indicator, to make
	// sure we go through this step exactly once
	// This can go away when the AI code is removed
	if pgame.Leader == -1 {
		pgame.Leader = 0

		var aerr *web.AppError
		err := datastore.RunInTransaction(c, func(tc appengine.Context) error {
			aerr = gameplay.StartPicking(tc, pgame)
			return nil
		}, nil)
		if err != nil {
			return nil, -1, &web.AppError{err, "Error starting game (transaction failed?)", 500}
		}
		if aerr != nil {
			return nil, -1, aerr
		}
	}

	mypos, aerr := JoinGame(c, session, pgame)
	if aerr != nil {
		return nil, -1, aerr
	}

	return pgame, mypos, nil
}

func JoinGame(c appengine.Context, session *sessions.Session, game *data.Game) (int, *web.AppError) {
	// This step is critical: here we validate that the authenticated
	// userID is a participant in the game, before we hand them a
	// cryptographic cookie with the game in it
	userID, _ := session.Values["userID"].(string)
	mypos, ok := game.LookupUserID(userID)
	if !ok {
		m := "Not a user in the current game"
		return -1, &web.AppError{errors.New(m), m, 500}
	}

	// Our participantID might have changed since the game started (if
	// we left and rejoined) so update it here
	participantID, _ := session.Values["participantID"].(string)
	if game.Players[mypos] != participantID {
		game.Players[mypos] = participantID
		db.StoreGame(c, *game)
	}

	session.Values["gameID"] = game.Id

	return mypos, nil
}

type GameReveal struct {
	Players []string `json:"players"`
	Label string `json:"label"`
}

func GetGameReveal(game data.Game, mypos int) []GameReveal {
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

	return reveals
}

func ValidateGameStart(session *sessions.Session, gamestartdata GameStartData) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	if len(gamestartdata.Participants) > 5 {
		m := "Cannot have more than five players"
		return &web.AppError{errors.New(m), m, 500}
	}

	return nil
}

func ReqGameStart(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	var gamestartdata GameStartData
	err := json.NewDecoder(r.Body).Decode(&gamestartdata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	aerr := ValidateGameStart(session, gamestartdata)
	if aerr != nil {
		return aerr
	}

	c := appengine.NewContext(r)
	pgame, mypos, aerr := DoGameStartOrJoin(c, session, game_factory(gamestartdata))
	if aerr != nil {
		return aerr
	}

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return gameplay.ReqGameState(w, r, c, session, *pgame, mypos)
}

func ValidateGameJoin(session *sessions.Session) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	return nil
}

func ReqGameJoin(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	aerr := ValidateGameJoin(session)
	if aerr != nil {
		return aerr
	}

	c := appengine.NewContext(r)
	pgame, mypos, aerr := DoGameStartOrJoin(c, session, nil)
	if aerr != nil {
		return aerr
	}

	err := session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return gameplay.ReqGameState(w, r, c, session, *pgame, mypos)
}

func ReqGameReveal(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, game data.Game, mypos int) *web.AppError {
	reveals := GetGameReveal(game, mypos)

	w.Header().Set("Content-type", "application/json")
	err := json.NewEncoder(w).Encode(&reveals)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}
