package metaid

import (
	"testing"
	"time"
)

func TestConnectRetryDelayStaysWithinJitterBand(t *testing.T) {
	for attempts := 0; attempts < 40; attempts++ {
		for i := 0; i < 200; i++ {
			d := ConnectRetryDelay(attempts)
			base := min(time.Duration(1<<min(attempts, 30))*time.Second, MaxConnectRetryInterval)
			lo, hi := time.Duration(float64(base)*0.8), time.Duration(float64(base)*1.2)
			if d < lo || d > hi {
				t.Fatalf("attempts=%d: delay %v outside [%v, %v]", attempts, d, lo, hi)
			}
		}
	}
}

func TestConnectRetryDelayIsCapped(t *testing.T) {
	if d := ConnectRetryDelay(60); d > time.Duration(float64(MaxConnectRetryInterval)*1.2) {
		t.Fatalf("attempts=60 not capped: %v", d)
	}
}

func TestConnectRetryDelayActuallyJitters(t *testing.T) {
	seen := map[time.Duration]bool{}
	for i := 0; i < 50; i++ {
		seen[ConnectRetryDelay(5)] = true
	}
	if len(seen) < 10 {
		t.Fatalf("expected varied delays, got %d distinct values", len(seen))
	}
}
