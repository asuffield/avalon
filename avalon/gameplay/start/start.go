package start

import (
	"appengine"
	"appengine/datastore"
	"avalon/data"
	"avalon/data/cards"
	"avalon/db"
	"avalon/db/trans"
	"avalon/gameplay"
	"avalon/gameplay/state"
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
	http.Handle("/game/setup", web.AjaxHandler(ReqGameSetup))
	http.Handle("/game/start", web.AjaxHandler(ReqGameStart))
	http.Handle("/game/join", web.AjaxHandler(ReqGameJoin))
	http.Handle("/game/reveal", web.GameHandler(ReqGameReveal))
}

type GameStartData struct {
	Participants map[string]string `json:"players"`
	Cards []string `json:"cards"`
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
	return func(gameid string, hangoutid string) (data.Game, []string) {
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

		setup := data.GetSizeSetup(len(players))
		setup.Cards = gamestartdata.Cards

		gamestatic := data.GameStatic{
			Id: gameid,
			Hangout: hangoutid,
			StartTime: time.Now(),
			UserIDs: ordered_participants,
			AIs: ais,
			Setup: setup,
			Roles: mathrand.Perm(len(players)),
		}
		gamestate := data.GameState{
			DataVersion: 1,

			HaveProposal: false,

			Leader: -1, // See comment in ReqGameStart - this is the "start of game" marker
			ThisProposal: 0,
			HaveActions: false,

			ThisMission: 0,
			MissionsComplete: make([]bool, len(gamestatic.Setup.Missions)),
			GoodScore: 0,
			EvilScore: 0,
			AssassinTarget: -1,
			GameOver: false,

			ThisVote: 0,
		}

		return data.Game{GameStatic: gamestatic, State: &gamestate}, players
	}
}

func DoStartGame(c appengine.Context, game *data.Game) *web.AppError {
	return trans.RunGameTransaction(c, game, func(tc appengine.Context, game data.Game) *web.AppError {
		// We use leader == -1 as a "start of game" indicator, to make
		// sure we go through this step exactly once
		// This can go away when the AI code is removed
		if game.State.Leader == -1 {
			game.State.Leader = 0

			err := db.StoreGameState(c, game)
			if err != nil {
				return &web.AppError{err, "Error storing game", 500}
			}

			return gameplay.StartPicking(tc, game)
		}
		return nil
	})
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

	aerr := DoStartGame(c, pgame)
	if aerr != nil {
		return nil, -1, aerr
	}

	mypos, aerr := JoinGame(c, session, *pgame)
	if aerr != nil {
		return nil, -1, aerr
	}

	return pgame, mypos, nil
}

func JoinGame(c appengine.Context, session *sessions.Session, game data.Game) (int, *web.AppError) {
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

	err := db.StorePlayerID(c, game, mypos, participantID)
	if err != nil {
		return -1, &web.AppError{err, "Error updating participant ID", 500}
	}

	session.Values["gameID"] = game.Id

	return mypos, nil
}

func GetGameReveal(game data.Game, mypos int) []data.GameReveal {
	myrole := game.Roles[mypos]
	mycard := game.Cards[myrole]

	reveals := make([]data.GameReveal, 1)

	reveals[0] = data.GameReveal{Label: "Your card: " + mycard.Label(), Players: []int{} }

	for _, reveal := range mycard.Reveal(game) {
		reveals = append(reveals, reveal)
	}

	return reveals
}

func ValidateGameStart(session *sessions.Session, gamestartdata GameStartData) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	participant_count := len(gamestartdata.Participants)
	if participant_count < 5 {
		// We will populate with AIs later
		participant_count = 5
	}

	setup := data.GetSizeSetup(participant_count)
	if len(setup.Missions) == 0 {
		m := "Invalid number of players"
		return &web.AppError{errors.New(m), m, 400}
	}

	if len(gamestartdata.Cards) != participant_count {
		m := "Mismatching number of players and cards"
		return &web.AppError{errors.New(m), m, 400}
	}

	cardCounts := map[string]int{}
	cardops := make([]data.CardOps, len(gamestartdata.Cards))
	for i, label := range gamestartdata.Cards {
		ctor, ok := cards.CardFactory[label]
		if !ok {
			m := "Invalid card " + label
			return &web.AppError{errors.New(m), m, 400}
		}

		cardops[i] = ctor()
		cardCounts[label] = cardCounts[label] + 1
	}

	assassin := cardops[0]
	evilCount := 0
	for _, card := range cardops {
		if card.Maximum() > 0 && card.Maximum() < cardCounts[card.Label()] {
			m := "Too many copies of " + card.Label()
			return &web.AppError{errors.New(m), m, 400}
		}

		if card.AllocatedAsSpy() {
			evilCount++
		}

		if card.AssassinPriority() > assassin.AssassinPriority() {
			assassin = card
		}
	}

	if evilCount != setup.Spies {
		m := "Wrong number of evil cards for this number of players"
		return &web.AppError{errors.New(m), m, 400}
	}

	_, haveMerlin := cardCounts["Merlin"]
	if haveMerlin && assassin.AssassinPriority() == 0 {
		m := "Must have an assassin with Merlin in play"
		return &web.AppError{errors.New(m), m, 400}
	}

	return nil
}

func ReqGameStart(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session) *web.AppError {
	var gamestartdata GameStartData
	err := json.NewDecoder(r.Body).Decode(&gamestartdata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	aerr := ValidateGameStart(session, gamestartdata)
	if aerr != nil {
		return aerr
	}

	pgame, mypos, aerr := DoGameStartOrJoin(c, session, game_factory(gamestartdata))
	if aerr != nil {
		return aerr
	}

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return state.ReqGameState(w, r, c, session, *pgame, mypos)
}

func ValidateGameJoin(session *sessions.Session) *web.AppError {
	userID, ok := session.Values["userID"].(string)
	if !ok || 0 == len(userID) {
		m := "Not authenticated via oauth"
		return &web.AppError{errors.New(m), m, 403}
	}

	return nil
}

func ReqGameJoin(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session) *web.AppError {
	aerr := ValidateGameJoin(session)
	if aerr != nil {
		return aerr
	}

	pgame, mypos, aerr := DoGameStartOrJoin(c, session, nil)
	if aerr != nil {
		return aerr
	}

	err := session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return state.ReqGameState(w, r, c, session, *pgame, mypos)
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

type GameSetupData struct {
	Players int `json:"players"`
}

type GameSetupResponse struct {
	Setup data.GameSetup `json:"setup"`
	GoodCards []string `json:"good_cards"`
	EvilCards []string `json:"evil_cards"`
}

func ReqGameSetup(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session) *web.AppError {
	var gamesetupdata GameSetupData
	err := json.NewDecoder(r.Body).Decode(&gamesetupdata)
	if err != nil {
		return &web.AppError{err, "Error parsing json body", 500}
	}

	setup := data.GetSizeSetup(gamesetupdata.Players)
	if len(setup.Missions) == 0 {
		m := "Invalid number of players"
		return &web.AppError{errors.New(m), m, 400}
	}

	goodCards := []string{}
	evilCards := []string{}
	for _, card := range cards.AllCards() {
		if card.AllocatedAsSpy() {
			evilCards = append(evilCards, card.Label())
		} else {
			goodCards = append(goodCards, card.Label())
		}
	}

	response := GameSetupResponse{setup, goodCards, evilCards}

	w.Header().Set("Content-type", "application/json")
	err = json.NewEncoder(w).Encode(&response)
	if err != nil {
		return &web.AppError{err, "Error encoding json", 500}
	}

	return nil
}
