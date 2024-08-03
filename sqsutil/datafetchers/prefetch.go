package datafetchers

import (
	"errors"
	"sync"
	"time"
)

// Fetcher is an interface that provides a method to get a value.
type Fetcher[T any] interface {
	Get() (T, time.Time, error)
	GetRefetchInterval() time.Duration
	WaitUntilFirstResult()
}

// IntervalFetcher is a struct that prefetches a value at a given interval
// and provides a method to get the latest value.
// NOTE: It may return stale data if the update function takes longer than the interval.
type IntervalFetcher[T any] struct {
	updateFn  func() T
	interval  time.Duration
	hasClosed bool

	hasSuccesfullyFetched bool
	firstFetchChan        chan struct{}
	lastRetrievedTime     time.Time
	cache                 T
	timer                 *time.Ticker
	mutex                 sync.RWMutex
}

func NewIntervalFetcher[T any](updateFn func() T, interval time.Duration) *IntervalFetcher[T] {
	if interval <= 0 {
		panic("interval must be greater than 0")
	}
	prefetcher := &IntervalFetcher[T]{
		updateFn:       updateFn,
		interval:       interval,
		firstFetchChan: make(chan struct{}),
	}

	go prefetcher.startTimer()

	return prefetcher
}

func (p *IntervalFetcher[T]) startTimer() {
	p.prefetch()
	p.timer = time.NewTicker(p.interval)

	for range p.timer.C {
		p.prefetch()
	}
}

func (p *IntervalFetcher[T]) prefetch() {
	newValue := p.updateFn()
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.hasSuccesfullyFetched {
		p.hasSuccesfullyFetched = true
		p.firstFetchChan <- struct{}{}
	}
	p.lastRetrievedTime = time.Now()
	p.cache = newValue
}

// WaitUntilFirstResult blocks until the first value is fetched.
// This function cannot be called concurrently multiple times.
func (p *IntervalFetcher[T]) WaitUntilFirstResult() {
	p.mutex.RLock()
	hasFetched := p.hasSuccesfullyFetched
	p.mutex.RUnlock()
	if hasFetched {
		return
	}
	<-p.firstFetchChan
	close(p.firstFetchChan)
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

	p.hasClosed = true
	p.timer.Stop()
}

func (p *IntervalFetcher[T]) GetRefetchInterval() time.Duration {
	return p.interval
}
