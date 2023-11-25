package workpool

import "sync"

type worker struct {
	workerPool chan *worker
	jobChannel chan Job
	stop       chan struct{}
}

type dispatcher struct {
	workerPool chan *worker
	jobQueue   chan Job
	stop       chan struct{}
}

type Job func()

type Pool struct {
	JobQueue   chan Job
	dispatcher *dispatcher
	wg         sync.WaitGroup
}

func (w *worker) start() {
	go func() {
		var job Job
		for {
			w.workerPool <- w
			select {
			case job = <-w.jobChannel:
				job()
			case <-w.stop:
				w.stop <- struct{}{}
				return
			}
		}
	}()
}

func newWorker(pool chan *worker) *worker {
	return &worker{
		workerPool: pool,
		jobChannel: make(chan Job),
		stop:       make(chan struct{}),
	}
}

func (d *dispatcher) dispatch() {
	for {
		select {
		case job := <-d.jobQueue:
			worker := <-d.workerPool
			worker.jobChannel <- job
		case <-d.stop:
			for i := 0; i < cap(d.workerPool); i++ {
				worker := <-d.workerPool
				worker.stop <- struct{}{}
				<-worker.stop
			}
			d.stop <- struct{}{}
			return
		}
	}
}

func newDispatcher(workerPool chan *worker, jobQueue chan Job) *dispatcher {
	d := &dispatcher{
		workerPool: workerPool,
		jobQueue:   jobQueue,
		stop:       make(chan struct{}),
	}
	for i := 0; i < cap(d.workerPool); i++ {
		worker := newWorker(d.workerPool)
		worker.start()
	}
	go d.dispatch()
	return d
}

func NewPool(numWorkers int, jobQueueLen int) *Pool {
	jobQ := make(chan Job, jobQueueLen)
	workerPool := make(chan *worker, numWorkers)

	return &Pool{
		JobQueue:   jobQ,
		dispatcher: newDispatcher(workerPool, jobQ),
	}
}

func (p *Pool) JobDone() {
	p.wg.Done()
}

func (p *Pool) WaitCount(count int) {
	p.wg.Add(count)
}

func (p *Pool) WaitAll() {
	p.wg.Wait()
}

func (p *Pool) Release() {
	p.dispatcher.stop <- struct{}{}
	<-p.dispatcher.stop
}
