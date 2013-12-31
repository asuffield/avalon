package web

import (
	"appengine"
	"avalon/data"
	"avalon/db"
	"avalon/web/keys"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
)

// Store initializes the Gorilla session store.
var Store = sessions.NewCookieStore([]byte(keys.CookieKey1Auth), []byte(keys.CookieKey1Encr))

type AppHandler func(http.ResponseWriter, *http.Request, appengine.Context, *sessions.Session) *AppError
type AjaxHandler func(http.ResponseWriter, *http.Request, appengine.Context, *sessions.Session) *AppError
type GameHandler func(http.ResponseWriter, *http.Request, appengine.Context, *sessions.Session, data.Game, int) *AppError

type AppError struct {
	Err     error
	Message string
	Code    int
}

// serveHTTP formats and passes up an error
func (fn AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session, _ := Store.Get(r, "sessionName")
	c := appengine.NewContext(r)

	if e := fn(w, r, c, session); e != nil {
		c.Errorf("%s: %s", e.Message, e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

func ajax_cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("origin"))
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "accept, content-type, cookie, x-csrf-token")
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

	state := session.Values["state"].(string)
	csrfToken  := r.Header.Get("x-csrf-token")
	if csrfToken != state {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}

	c := appengine.NewContext(r)

	if e := fn(w, r, c, session); e != nil { // e is *AppError, not os.Error.
		c.Errorf("%s: %s", e.Message, e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

func gameSetup(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session, mygame *data.Game, mypos *int) *AppError {
	gameID, ok := session.Values["gameID"].(string)
	if !ok || 0 == len(gameID) {
		m := "Not in a game"
		return &AppError{errors.New(m), m, 403}
	}

	userID, _ := session.Values["userID"].(string)
	hangoutID, _ := session.Values["hangoutID"].(string)

	game, err := db.RetrieveGame(c, hangoutID, gameID)
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

	pos, ok := game.LookupUserID(userID)
	if !ok {
		m := "Not a user in that game"
		return &AppError{errors.New(m), m, 500}
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

	state := session.Values["state"].(string)
	csrfToken  := r.Header.Get("x-csrf-token")
	if csrfToken != state {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}

	c := appengine.NewContext(r)
	var game data.Game
	var mypos int
	e := gameSetup(w, r, c, session, &game, &mypos)
	if e != nil {
		c.Errorf("%s: %s", e.Message, e.Err)
		http.Error(w, e.Message, e.Code)
		return
	}

	if e = fn(w, r, c, session, game, mypos); e != nil { // e is *AppError, not os.Error.
		c.Errorf("%s: %s", e.Message, e.Err)
		http.Error(w, e.Message, e.Code)
	}
}

