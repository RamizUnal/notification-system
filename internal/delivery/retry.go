package delivery

import (
	"math/rand/v2"
	"time"
)

// ExponentialBackoff spaces retries exponentially with "equal jitter": the
// pause after the n-th failed attempt is a random duration between half and
// all of min(Max, Base·2^(n-1)).
//
// The random half spreads out notifications that failed together (for
// example during a provider outage) so they do not come back as a thundering
// herd; the fixed half guarantees a minimum pause so a retry never fires
// immediately after the failure that caused it.
type ExponentialBackoff struct {
	Base time.Duration
	Max  time.Duration
	// Rand returns a number in [0, 1). Nil means math/rand/v2.
	Rand func() float64
}

// Delay returns the pause after the given (1-based) failed attempt.
func (b ExponentialBackoff) Delay(attempt int) time.Duration {
	ceiling := b.Base
	for i := 1; i < attempt && ceiling < b.Max; i++ {
		ceiling *= 2
	}
	ceiling = min(ceiling, b.Max)

	random := rand.Float64
	if b.Rand != nil {
		random = b.Rand
	}
	half := ceiling / 2
	return half + time.Duration(random()*float64(ceiling-half))
}
