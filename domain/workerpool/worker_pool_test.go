// Note: all tests in this file are generated via ChatGPT.
package workerpool_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/osmosis-labs/sqs/domain/workerpool"
)

func TestWorkerStartAndStop(t *testing.T) {
	workerPool := make(chan chan Job[int], 1)
	resultQueue := make(chan JobResult[int], 1)

	worker := NewWorker(1, workerPool, resultQueue)
	var wg sync.WaitGroup
	wg.Add(1)
	worker.Start(&wg)

	select {
	case <-workerPool:
		// Worker registered itself to the pool
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not register to the pool in time")
	}

	worker.Stop()
	wg.Wait()
}

func TestJobExecution(t *testing.T) {
	workerPool := make(chan chan Job[int], 1)
	resultQueue := make(chan JobResult[int], 1)

	worker := NewWorker(1, workerPool, resultQueue)
	var wg sync.WaitGroup
	wg.Add(1)
	worker.Start(&wg)

	// Create the job
	job := Job[int]{Task: func() (int, error) { return 42, nil }}

	// Register the worker to the worker pool
	workerChannel := <-workerPool

	// Dispatch the job to the worker
	workerChannel <- job

	select {
	case result := <-resultQueue:
		if result.Result != 42 {
			t.Errorf("expected 42, got %d", result.Result)
		}
		if result.Err != nil {
			t.Errorf("expected no error, got %v", result.Err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("job result was not received in time")
	}

	worker.Stop()
	wg.Wait()
}

func TestJobExecutionWithError(t *testing.T) {
	workerPool := make(chan chan Job[int], 1)
	resultQueue := make(chan JobResult[int], 1)

	worker := NewWorker(1, workerPool, resultQueue)
	var wg sync.WaitGroup
	wg.Add(1)
	worker.Start(&wg)

	// Create the job with an error
	job := Job[int]{Task: func() (int, error) { return 0, errors.New("test error") }}

	// Register the worker to the worker pool
	workerChannel := <-workerPool

	// Dispatch the job to the worker
	workerChannel <- job

	select {
	case result := <-resultQueue:
		if result.Err == nil {
			t.Fatal("expected error, got nil")
		}
		if result.Err.Error() != "test error" {
			t.Errorf("expected 'test error', got %v", result.Err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("job result was not received in time")
	}

	worker.Stop()
	wg.Wait()
}

func TestDispatcherRun(t *testing.T) {
	dispatcher := NewDispatcher[int](1)

	// Create a wait group to wait for the dispatcher to finish
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		dispatcher.Run()
	}()

	// Wait for a short period to ensure all workers are started
	time.Sleep(100 * time.Millisecond)

	job := Job[int]{Task: func() (int, error) { return 99, nil }}

	dispatcher.JobQueue <- job

	select {
	case result := <-dispatcher.ResultQueue:
		if result.Result != 99 {
			t.Errorf("expected 99, got %d", result.Result)
		}
		if result.Err != nil {
			t.Errorf("expected no error, got %v", result.Err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("job result was not received in time")
	}

	// Stop the dispatcher by closing the JobQueue and wait for it to finish
	dispatcher.Stop()
	wg.Wait()
}
