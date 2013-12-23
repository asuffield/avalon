package dump

import (
	"appengine"
	"avalon/data"
	"avalon/db"
	"avalon/web"
	"errors"
	"github.com/gorilla/sessions"
	"html/template"
	"net/http"
)

type DumpProposal struct {
	Proposal *data.Proposal
	Votes map[int]bool
}

type DumpMission struct {
	Proposals []DumpProposal
	Mission *int
	Actions map[int]bool
}

type DumpGameData struct {
	Missions []DumpMission
	Results []data.MissionResult
	Game data.Game
}

var dumpGameTemplate = template.Must(template.ParseFiles("template/dumpgame.html"))

func DumpGame(w http.ResponseWriter, r *http.Request, session *sessions.Session) *web.AppError {
	hangout := r.FormValue("hangout")
	gameid := r.FormValue("game")

	c := appengine.NewContext(r)
	pgame, _ := db.RetrieveGame(c, hangout, gameid)
	if pgame == nil {
		m := "Could not find game"
		return &web.AppError{errors.New(m), m, 404}
	}
	game := *pgame

	missions := make([]DumpMission, 5)
	for m := 0; m < 5; m++ {
		missions[m].Proposals = make([]DumpProposal, 5)
		missions[m].Mission, _ = db.GetMission(c, game, m)
		missions[m].Actions, _ = db.GetActions(c, game, m)
		for p := 0; p < 5; p++ {
			missions[m].Proposals[p].Proposal, _ = db.GetProposal(c, game, m, p)
			missions[m].Proposals[p].Votes, _ = db.GetVotes(c, game, m, p)
		}
	}

	results, _ := db.GetMissionResults(c, game)

	dump := DumpGameData{ Game: game, Missions: missions, Results: results }

	w.Header().Set("Content-Type", "text/html")
	err := dumpGameTemplate.Execute(w, dump)
	if err != nil {
		return &web.AppError{err, "Error rendering template", 500}
	}

	return nil
}
