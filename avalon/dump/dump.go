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

func init() {
	http.Handle("/admin/dumpgame", web.AppHandler(ReqDumpGame))
}

type DumpProposal struct {
	Proposal *data.Proposal
}

type DumpMission struct {
	Proposals []DumpProposal
	Actions *data.Actions
}

type DumpGameData struct {
	Missions []DumpMission
	MissionResults []*data.MissionResult
	VoteResults []data.VoteResult
	PlayerIDs []string
	Game data.Game
}

var recentGamesTemplate = template.Must(template.ParseFiles("template/recentgames.html"))
var dumpGameTemplate = template.Must(template.ParseFiles("template/dumpgame.html"))

func ReqDumpGame(w http.ResponseWriter, r *http.Request, c appengine.Context, session *sessions.Session) *web.AppError {
	hangout := r.FormValue("hangout")
	gameid := r.FormValue("game")

	if hangout == "" || gameid == "" {
		games, err := db.RecentGames(c, 20)
		if err != nil {
			return &web.AppError{err, "Could not retrieve recent games", 500}
		}

		w.Header().Set("Content-Type", "text/html")
		err = recentGamesTemplate.Execute(w, games)
		if err != nil {
			return &web.AppError{err, "Error rendering template", 500}
		}
		return nil
	}

	pgame, err := db.RetrieveGame(c, hangout, gameid)
	if err != nil {
		return &web.AppError{err, "Could not retrieve game", 500}
	}
	if pgame == nil {
		m := "Could not find game"
		return &web.AppError{errors.New(m), m, 404}
	}
	err = db.EnsureGameState(c, pgame, true)
	if err != nil {
		return &web.AppError{err, "Could not retrieve game state", 500}
	}
	game := *pgame

	missions := make([]DumpMission, 5)
	for m := 0; m < 5; m++ {
		missions[m].Proposals = make([]DumpProposal, 5)
		missions[m].Actions, _ = db.GetActions(c, true, game, m)
		for p := 0; p < 5; p++ {
			missions[m].Proposals[p].Proposal, _ = db.GetProposal(c, true, game, m, p)
		}
	}

	playerids, _ := db.GetPlayerIDs(c, game)
	missionresults, _ := db.GetMissionResults(c, game)
	voteresults, _ := db.GetVoteResults(c, game)

	dump := DumpGameData{
		Game: game,
		PlayerIDs: playerids,
		Missions: missions,
		MissionResults: missionresults,
		VoteResults: voteresults,
	}

	w.Header().Set("Content-Type", "text/html")
	err = dumpGameTemplate.Execute(w, dump)
	if err != nil {
		return &web.AppError{err, "Error rendering template", 500}
	}

	return nil
}
