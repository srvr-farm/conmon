package scheduler

import (
	"context"
	"sync"
	"time"
)

type Job struct {
	Interval time.Duration
	Run      func(context.Context)
}

type Scheduler struct {
	clock Clock
}

func New(clock Clock) *Scheduler {
	if clock == nil {
		clock = NewRealClock()
	}
	return &Scheduler{clock: clock}
}

func (s *Scheduler) Start(ctx context.Context, jobs []Job) <-chan struct{} {
	done := make(chan struct{})
	if len(jobs) == 0 {
		close(done)
		return done
	}

	var wg sync.WaitGroup
	wg.Add(len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			defer wg.Done()
			s.runJob(ctx, job)
		}()
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	return done
}

func (s *Scheduler) runJob(ctx context.Context, job Job) {
	job.Run(ctx)

	ticker := s.clock.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			job.Run(ctx)
		}
	}
}
