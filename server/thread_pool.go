package main

const BufferSize uint16 = 64

type Task func()

type ThreadPool struct {
	tasks chan Task
}

func NewThreadPool(numWorkers int) *ThreadPool {
	pool := &ThreadPool{tasks: make(chan Task, BufferSize)}
	for range numWorkers {
		go pool.worker()
	}
	return pool
}

func (p *ThreadPool) worker() {
	for {
		task := <-p.tasks
		task()
	}
}

func (p *ThreadPool) submit(task Task) {
	p.tasks <- task
}
