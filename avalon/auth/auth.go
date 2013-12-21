package auth

import (
	"appengine"
	"appengine/urlfetch"
	"avalon/web"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gorilla/sessions"
	"net/http"
	"net/url"
    "log"
	"text/template"
)

const (
	clientID = "834761542099-061td9hu3vl1mochrijcvrrt1e4egvq9.apps.googleusercontent.com"
)

var appjsTemplate = template.Must(template.ParseFiles("template/app.js"))

type AuthData struct {
	Token string `json:"token"`
	MyId string `json:"myid"`
	Hangout string `json:"hangout"`
	HangoutUrl string `json:"hangouturl"`
}

type TokenInfo struct {
	IssuedTo string `json:"issued_to"`
	Audience string `json:"audience"`
	UserId string `json:"user_id"`
	Scope string `json:"scope"`
	ExpiresIn int `json:"expires_in"`
	AccessType string `json:"access_type"`
}

// randomString returns a random string with the specified length
func RandomString(length int) (str string) {
	b := make([]byte, length)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func AppJS(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	// Create a state token to prevent request forgery and store it in the session
	// for later validation
	state := RandomString(64)
	session.Values["state"] = state
	err := session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	stateURL := url.QueryEscape(session.Values["state"].(string))

	// Fill in the missing fields in index.html
	var data = struct {
		ClientID, State string
	}{clientID, stateURL}

	// Render and serve the HTML
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/javascript")
	err = appjsTemplate.Execute(w, data)
	if err != nil {
		return &web.AppError{err, "Error rendering template", 500}
	}
	return nil
}

func fetch_token_info(c appengine.Context, token string) (*TokenInfo, *web.AppError) {
	client := urlfetch.Client(c)
	addr := "https://www.googleapis.com/oauth2/v1/tokeninfo"
	values := url.Values{
		"access_token": {token},
	}
	resp, err := client.PostForm(addr, values)
	if err != nil {
		return nil, &web.AppError{err, "Error validating access token", 500}
	}
	defer resp.Body.Close()

	var tokeninfo TokenInfo
	err = json.NewDecoder(resp.Body).Decode(&tokeninfo)
	if err != nil {
		return nil, &web.AppError{err, "Error decoding tokeninfo", 500}
	}

	return &tokeninfo, nil
}

func AuthToken(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	state := session.Values["state"].(string)

	if r.FormValue("state") != state {
		m := "Invalid state parameter"
		return &web.AppError{errors.New(m), m, 403}
	}
	delete(session.Values, "state")

	var authdata AuthData
	err := json.NewDecoder(r.Body).Decode(&authdata)
	if err != nil {
		return &web.AppError{err, "Error storing parsing json body", 500}
	}

	c := appengine.NewContext(r)
	tokeninfo, aerr := fetch_token_info(c, authdata.Token)
	if aerr != nil {
		return aerr
	}

	log.Printf("Got userID %s and hangoutID %s", tokeninfo.UserId, authdata.Hangout)

	session.Values["userID"] = tokeninfo.UserId
	session.Values["participantID"] = authdata.MyId
	session.Values["hangoutID"] = authdata.Hangout

	err = session.Save(r, w)
	if err != nil {
		log.Println("error saving session:", err)
	}

	return nil
}
