package datafetchers

import (
	"errors"
	"sync"
	"time"
)

// defaultEmptyCacheRetryInterval is how long to wait before retrying a failed fetch
// while no value has ever been retrieved. Without it, a failed first fetch after startup
// leaves the cache empty for a whole refetch interval.
const defaultEmptyCacheRetryInterval = 15 * time.Second

// Fetcher is an interface that provides a method to get a value.
type Fetcher[T any] interface {
	Get() (T, time.Time, error)
	GetRefetchInterval() time.Duration
}

// IntervalFetcher is a struct that prefetches a value at a given interval
// and provides a method to get the latest value.
// NOTE: It may return stale data if the update function takes longer than the interval.
// A failed refresh keeps the previously retrieved value.
type IntervalFetcher[T any] struct {
	updateFn                func() (T, error)
	interval                time.Duration
	emptyCacheRetryInterval time.Duration
	hasClosed               bool

	lastRetrievedTime time.Time
	cache             T
	timer             *time.Ticker
	// done is closed by Close to stop the refresh loop.
	done  chan struct{}
	mutex sync.RWMutex
}

func NewIntervalFetcher[T any](updateFn func() (T, error), interval time.Duration) *IntervalFetcher[T] {
	return newIntervalFetcher(updateFn, interval, defaultEmptyCacheRetryInterval)
}

func newIntervalFetcher[T any](updateFn func() (T, error), interval, emptyCacheRetryInterval time.Duration) *IntervalFetcher[T] {
	if interval <= 0 {
		panic("interval must be greater than 0")
	}
	prefetcher := &IntervalFetcher[T]{
		updateFn:                updateFn,
		interval:                interval,
		emptyCacheRetryInterval: min(emptyCacheRetryInterval, interval),
		done:                    make(chan struct{}),
	}

	go prefetcher.startTimer()

	return prefetcher
}

func (p *IntervalFetcher[T]) startTimer() {
	// Retry on the short interval until the first value is retrieved.
	for !p.prefetch() {
		select {
		case <-time.After(p.emptyCacheRetryInterval):
		case <-p.done:
			return
		}
	}

	// The ticker is created under the lock so that Close either stops it
	// or, if it runs first, prevents it from being started.
	p.mutex.Lock()
	if p.hasClosed {
		p.mutex.Unlock()
		return
	}
	timer := time.NewTicker(p.interval)
	p.timer = timer
	p.mutex.Unlock()

	for {
		select {
		case <-timer.C:
			p.prefetch()
		case <-p.done:
			return
		}
	}
}

// prefetch refreshes the cache and reports whether the refresh succeeded.
func (p *IntervalFetcher[T]) prefetch() bool {
	newValue, err := p.updateFn()
	if err != nil {
		// By silently skipping the error, the values would become stale,
		// signaling that to the client.
		return false
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.lastRetrievedTime = time.Now()
	p.cache = newValue
	return true
}

// Returns the latest value and the time it was last retrieved.
// If no value has ever been retrieved, it returns the zero value of T and time.Time{}.
// If p.hasClosed is true, it returns the zero value of T and time.Time{}.
func (p *IntervalFetcher[T]) Get() (T, time.Time, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.lastRetrievedTime.IsZero() {
		return p.cache, time.Time{}, errors.New("no cached value has ever been retrieved")
	}
	if p.hasClosed {
		return p.cache, time.Time{}, errors.New("prefetcher has been closed")
	}

	return p.cache, p.lastRetrievedTime, nil
}

func (p *IntervalFetcher[T]) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.hasClosed {
		return
	}
	p.hasClosed = true
	close(p.done)
	// The timer is nil until the first value is retrieved.
	if p.timer != nil {
		p.timer.Stop()
	}
}

func (p *IntervalFetcher[T]) GetRefetchInterval() time.Duration {
	return p.interval
}
