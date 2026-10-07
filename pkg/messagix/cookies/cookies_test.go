package cookies

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
)

const concurrencyIterations = 500

// Cookies is stored in the user login metadata, so it gets marshaled from
// whichever goroutine calls UserLogin.Save while HTTP responses keep updating
// it. Run both at once to make sure the map is never touched without the lock.
func TestCookiesConcurrentMarshalAndUpdate(t *testing.T) {
	c := &Cookies{}
	c.UpdateValues(map[MetaCookieName]string{IGCookieSessionID: "session"})

	errs := make(chan error, concurrencyIterations)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range concurrencyIterations {
			c.Set(IGCookieRUR, strconv.Itoa(i))
		}
	}()
	go func() {
		defer wg.Done()
		for range concurrencyIterations {
			if _, err := json.Marshal(c); err != nil {
				errs <- err
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range concurrencyIterations {
			if err := json.Unmarshal([]byte(`{"sessionid":"other"}`), c); err != nil {
				errs <- err
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCookiesConcurrentWWWClaim(t *testing.T) {
	c := &Cookies{}
	c.UpdateValues(nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range concurrencyIterations {
			c.SetWWWClaim(strconv.Itoa(i))
		}
	}()
	go func() {
		defer wg.Done()
		for range concurrencyIterations {
			c.GetWWWClaim()
		}
	}()
	wg.Wait()
}
