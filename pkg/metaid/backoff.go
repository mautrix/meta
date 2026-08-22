package metaid

import (
	"math/rand/v2"
	"time"
)

// MaxConnectRetryInterval caps how long a connect retry will ever wait. Without
// a cap, 1<<attempts seconds outgrows any sane number within a couple of dozen
// failures.
var MaxConnectRetryInterval = 5 * time.Minute

// ConnectRetryDelay returns how long to sleep before the next connection
// attempt: exponential in the attempt count, capped, with ±20% jitter.
//
// The jitter is the part that matters for a bridge serving many logins. Every
// login that fails at the same moment — a shared network blip, a process
// restart — would otherwise retry in perfect lockstep at 2s, 4s, 8s ... for the
// rest of its life, which is both a thundering herd against the remote service
// and indistinguishable from coordinated automated traffic.
func ConnectRetryDelay(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 30 {
		attempts = 30
	}
	base := min(time.Duration(1<<attempts)*time.Second, MaxConnectRetryInterval)
	jitter := time.Duration((rand.Float64()*0.4 - 0.2) * float64(base))
	return base + jitter
}
