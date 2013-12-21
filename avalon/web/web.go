package web

import (
	"appengine"
	"avalon/data"
	"avalon/db"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
    "log"
)

// Store initializes the Gorilla session store.
var Store = sessions.NewCookieStore([]byte("AhshaechieFoo6AeSh8ophoosuecooti"), []byte("nohb6aemooBo3aeyi5aem9cee2quauch"))

type AppHandler func(http.ResponseWriter, *http.Request, *sessions.Session) *AppError
type AjaxHandler func(http.ResponseWriter, *http.Request, *sessions.Session) *AppError
type GameHandler func(http.ResponseWriter, *http.Request, appengine.Context, *sessions.Session, data.Game, int) *AppError

type AppError struct {
	Err     error
	Message string
	Code    int
}

// serveHTTP formats and passes up an error
func (fn AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session, _ := Store.Get(r, "sessionName")

	if e := fn(w, r, session); e != nil { // e is *AppError, not os.Error.
		log.Println(e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

func ajax_cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("origin"))
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "accept, content-type, cookie")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

func (fn AjaxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ajax_cors(w, r) {
		return
	}

	session, _ := Store.Get(r, "sessionName")

	if e := fn(w, r, session); e != nil { // e is *AppError, not os.Error.
		log.Println(e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

func gameSetup(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, mygame *data.Game, mypos *int) *AppError {
	gameID, ok := session.Values["gameID"].(string)
	if !ok || 0 == len(gameID) {
		m := "Not in a game"
		return &AppError{errors.New(m), m, 403}
	}

	hangoutID, _ := session.Values["hangoutID"].(string)
	participantID, _ := session.Values["participantID"].(string)

	game, err := db.RetrieveGame(c, gameID)
	if err != nil {
		return &AppError{err, "Error fetching game from datastore", 500}
	}
	if game == nil {
		m := "Invalid gameid"
		return &AppError{errors.New(m), m, 404}
	}

	if game.Hangout != hangoutID {
		m := "Incorrect hangout for gameid"
		return &AppError{errors.New(m), m, 403}
	}

	playermap := data.MakePlayerMap(game.Players)
	pos, ok := playermap[participantID]
	if !ok {
		m := "Not a player in that game"
		return &AppError{errors.New(m), m, 403}
	}

	*mygame = *game
	*mypos = pos

	return nil
}

func (fn GameHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ajax_cors(w, r) {
		return
	}

	session, _ := Store.Get(r, "sessionName")
	c := appengine.NewContext(r)
	var game data.Game
	var mypos int
	e := gameSetup(w, r, c, session, &game, &mypos)
	if e != nil {
		log.Println(e.Err)
		http.Error(w, e.Message, e.Code)
		return
	}

	if e = fn(w, r, c, session, game, mypos); e != nil { // e is *AppError, not os.Error.
		log.Println(e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

