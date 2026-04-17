package firehose

import (
	"sync"
)

type ConcurrentFlushQueue struct {
	bufferSize int

	startSignal    chan uint64
	jobQueue       chan *blockOutput
	printBlockFunc func(*blockOutput)
	flushFunc      func()

	jobWG     sync.WaitGroup
	closeOnce sync.Once
}

func NewConcurrentFlushQueue(bufferSize int, printBlockFunc func(*blockOutput), flushFunc func()) *ConcurrentFlushQueue {
	return &ConcurrentFlushQueue{
		startSignal:    make(chan uint64, 1),
		jobQueue:       make(chan *blockOutput, bufferSize),
		bufferSize:     bufferSize,
		printBlockFunc: printBlockFunc,
		flushFunc:      flushFunc,
	}
}

func (q *ConcurrentFlushQueue) Start() {
	// Start a single worker for block serialization
	// (multiple workers would race on the output buffer)
	q.jobWG.Add(1)
	go q.worker()
}

func (q *ConcurrentFlushQueue) Push(out *blockOutput) {
	q.jobQueue <- out
}

// Close signals the worker to shut down and waits for it to finish
// It blocks until all concurrent block flushing operations are completed, ensuring a clean
// shutdown of the printing pipeline.
func (q *ConcurrentFlushQueue) Close() {
	q.closeOnce.Do(func() {
		close(q.jobQueue)
		q.jobWG.Wait()
	})
}

// worker listens for blocks and serializes them sequentially
func (q *ConcurrentFlushQueue) worker() {
	defer q.jobWG.Done()
	for out := range q.jobQueue {
		q.printBlockFunc(out)
		q.flushFunc()
	}
}
