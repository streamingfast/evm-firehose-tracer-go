package firehose

import (
	"sync"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// flushJob carries a block together with the LIB number computed at the time it
// was enqueued. The LIB must be captured here (not recomputed by the worker)
// because the tracer resets its finality state as soon as the block is pushed.
type flushJob struct {
	block  *pbeth.Block
	libNum uint64
}

type ConcurrentFlushQueue struct {
	bufferSize int

	startSignal    chan uint64
	jobQueue       chan flushJob
	printBlockFunc func(block *pbeth.Block, libNum uint64)
	flushFunc      func()

	jobWG     sync.WaitGroup
	closeOnce sync.Once
}

func NewConcurrentFlushQueue(bufferSize int, printBlockFunc func(*pbeth.Block, uint64), flushFunc func()) *ConcurrentFlushQueue {
	return &ConcurrentFlushQueue{
		startSignal:    make(chan uint64, 1),
		jobQueue:       make(chan flushJob, bufferSize),
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

func (q *ConcurrentFlushQueue) Push(block *pbeth.Block, libNum uint64) {
	q.jobQueue <- flushJob{block: block, libNum: libNum}
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
	for job := range q.jobQueue {
		q.printBlockFunc(job.block, job.libNum)
		q.flushFunc()
	}
}
