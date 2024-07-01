package workerpool

import "sync"

// Job represents the job to be run
type Job[T any] struct {
	Task func() (T, error)
}

// Result represents the result of a job
type JobResult[T any] struct {
	Result T
	Err    error
}

// Worker represents the worker that executes the job
type Worker[T any] struct {
	ID          int
	WorkerPool  chan chan Job[T]
	JobChannel  chan Job[T]
	ResultQueue chan<- JobResult[T]
	QuitChan    chan bool
}

func NewWorker[T any](id int, workerPool chan chan Job[T], resultQueue chan<- JobResult[T]) Worker[T] {
	return Worker[T]{
		ID:          id,
		WorkerPool:  workerPool,
		JobChannel:  make(chan Job[T]),
		ResultQueue: resultQueue,
		QuitChan:    make(chan bool),
	}
}

func (w Worker[T]) Start(wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		for {
			// Register the current worker into the worker queue
			w.WorkerPool <- w.JobChannel

			select {
			case job := <-w.JobChannel:
				// Execute job
				jobResult, err := job.Task()
				result := JobResult[T]{Result: jobResult, Err: err}

				// Send result to result queue
				w.ResultQueue <- result
			case <-w.QuitChan:
				// Quit the worker
				close(w.JobChannel)
				close(w.QuitChan)
				return
			}
		}
	}()
}

func (w Worker[T]) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}

type Dispatcher[T any] struct {
	WorkerPool  chan chan Job[T]
	MaxWorkers  int
	JobQueue    chan Job[T]
	ResultQueue chan JobResult[T]
	Workers     []Worker[T]
}

func NewDispatcher[T any](maxWorkers int) *Dispatcher[T] {
	workerPool := make(chan chan Job[T], maxWorkers)
	return &Dispatcher[T]{
		WorkerPool:  workerPool,
		MaxWorkers:  maxWorkers,
		JobQueue:    make(chan Job[T]),
		ResultQueue: make(chan JobResult[T]),
		Workers:     make([]Worker[T], maxWorkers),
	}
}

func (d *Dispatcher[T]) Run() {
	var wg sync.WaitGroup
	for i := 0; i < d.MaxWorkers; i++ {
		worker := NewWorker(i+1, d.WorkerPool, d.ResultQueue)
		wg.Add(1)
		worker.Start(&wg)

		d.Workers[i] = worker
	}

	go d.dispatch()

	wg.Wait()
	close(d.ResultQueue)
	close(d.JobQueue)
}

func (d *Dispatcher[T]) Stop() {
	for i := 0; i < d.MaxWorkers; i++ {
		d.Workers[i].Stop()
	}
}

func (d *Dispatcher[T]) dispatch() {
	for job := range d.JobQueue {
		go func(job Job[T]) {
			jobChannel := <-d.WorkerPool
			jobChannel <- job
		}(job)
	}
}
