package trans

import (
	"appengine"
	"appengine/datastore"
	"avalon/data"
	"avalon/db"
	"avalon/web"
)

type GameTransaction func(c appengine.Context, game data.Game) (*web.AppError)

func RunGameTransaction(c appengine.Context, game *data.Game, trans GameTransaction) *web.AppError {
	if game.State != nil {
		game.State = nil
	}
	var aerr *web.AppError
	err := datastore.RunInTransaction(c, func(tc appengine.Context) error {
		terr := db.EnsureGameState(c, game, true)
		if terr != nil {
			return terr
		}
		aerr = trans(tc, *game)
		if aerr != nil {
			// This makes the transaction abort
			return aerr.Err
		}
		return nil
	}, nil)
	if aerr != nil {
		return aerr
	}
	if err != nil {
		return &web.AppError{err, "Game transaction failed", 500}
	}
	err = db.FlushGameStateCache(c, *game)
	if err != nil {
		return &web.AppError{err, "Failed to flush game state cache after transaction", 500}
	}
	return nil
}

