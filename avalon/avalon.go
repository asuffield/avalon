package avalon

import (
	"time"
	mathrand "math/rand"
)

func init() {
	mathrand.Seed( time.Now().UTC().UnixNano())
}
