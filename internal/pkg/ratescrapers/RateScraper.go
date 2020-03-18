package ratescrapers

import (
	"errors"
	"sync"
	"time"

	models "github.com/diadata-org/diadata/pkg/model"
	log "github.com/sirupsen/logrus"
)

const (
	// Determine frequency of scraping
	refreshDelay = time.Second * 10 * 1
)

type nothing struct{}

type RateScraper struct {
	// signaling channels
	shutdown     chan nothing
	shutdownDone chan nothing

	// error handling; to read error or closed, first acquire read lock
	// only cleanup method should hold write lock
	errorLock        sync.RWMutex
	error            error
	closed           bool
	ticker           *time.Ticker
	datastore        models.Datastore
	chanInterestRate chan *models.InterestRate
}

// SpawnRateScraper returns a new RateScraper initialized with default values.
// The instance is asynchronously scraping as soon as it is created.
func SpawnRateScraper(datastore models.Datastore, scrapeType string) *RateScraper {
	s := &RateScraper{
		shutdown:         make(chan nothing),
		shutdownDone:     make(chan nothing),
		error:            nil,
		ticker:           time.NewTicker(refreshDelay),
		datastore:        datastore,
		chanInterestRate: make(chan *models.InterestRate),
	}

	log.Info("Rate scraper is built and triggered")
	go s.mainLoop(scrapeType)
	return s
}

// mainLoop runs in a goroutine until channel s is closed.
func (s *RateScraper) mainLoop(scrapeType string) {
	for {
		select {
		case <-s.ticker.C:
			s.Update(scrapeType)
		case <-s.shutdown: // user requested shutdown
			log.Println("RateScraper shutting down")
			s.cleanup(nil)
			return
		}
	}
}

// closes all connected Scrapers
// must only be called from mainLoop
func (s *RateScraper) cleanup(err error) {

	s.errorLock.Lock()
	defer s.errorLock.Unlock()

	s.ticker.Stop()

	if err != nil {
		s.error = err
	}
	s.closed = true

	close(s.shutdownDone) // signal that shutdown is complete
}

// Close closes any existing API connections
func (s *RateScraper) Close() error {
	if s.closed {
		return errors.New("RateScraper: Already closed")
	}
	close(s.shutdown)
	<-s.shutdownDone
	s.errorLock.RLock()
	defer s.errorLock.RUnlock()
	return s.error
}

// Channel returns a channel that can be used to receive rate information
func (s *RateScraper) Channel() chan *models.InterestRate {
	return s.chanInterestRate
}

func (s *RateScraper) Update(rateType string) error {
	// Returns the appropriate update function corresponding to the rate type.
	switch rateType {
	case "ESTER":
		return s.UpdateESTER()
	case "SOFR":
		return s.UpdateSOFR()
	}
	return errors.New(rateType + " does not exist in database")
}