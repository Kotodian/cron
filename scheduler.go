package cron

import (
	"sort"
	"time"
)

type Scheduler struct {
	jobs []*Job
	size int
}

func (s *Scheduler) Len() int {
	return s.size
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].nextRun.Unix() >= s.jobs[i].nextRun.Unix()
}

func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

var (
	defaultScheduler = NewScheduler()
)

func NewScheduler() *Scheduler {
	return &Scheduler{
		jobs: make([]*Job, 100),
		size: 0,
	}
}

func (s *Scheduler) Jobs() []*Job {
	return s.jobs[:s.size]
}

func (s *Scheduler) getRunnableJobs() (runningJobs []*Job, n int) {
	runnableJobs := make([]*Job, 100)
	n = 0
	sort.Sort(s)
	for i := 0; i < s.size; i++ {
		if s.jobs[i].shouldRun() {
			runnableJobs[n] = s.jobs[i]
			n++
		} else {
			break
		}
	}
	return runnableJobs, n
}

func (s *Scheduler) NextRun() (*Job, time.Time) {
	if s.size <= 0 {
		return nil, time.Now()
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].nextRun
}

func (s *Scheduler) Every(interval uint64) *Job {
	job := NewJob(interval)
	s.jobs[s.size] = job
	s.size++
	return job
}

func (s *Scheduler) RunPending() {
	runnableJobs, n := s.getRunnableJobs()
	if n != 0 {
		for i := 0; i < n; i++ {
			go runnableJobs[i].run()
			runnableJobs[i].lastRun = time.Now()
			runnableJobs[i].scheduleNextRun()
		}
	}
}

func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

func (s *Scheduler) RunAllWithDelay(d int) {
	for i := 0; i < s.size; i++ {
		go s.jobs[i].run()
		if d != 0 {
			time.Sleep(time.Duration(d))
		}
	}
}

func (s *Scheduler) Remove(j interface{}) {
	s.removeByCondition(func(job *Job) bool {
		return job.jobFunc == getFunctionName(j)
	})
}

func (s *Scheduler) RemoveByRef(j *Job) {
	s.removeByCondition(func(job *Job) bool {
		return job == j
	})
}

func (s *Scheduler) Scheduled(j interface{}) bool {
	for _, job := range s.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

func (s *Scheduler) Start() chan bool {
	stopped := make(chan bool, 1)
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.RunPending()
			case <-stopped:
				ticker.Stop()
				return
			}
		}
	}()
	return stopped
}

func (s *Scheduler) Clear() {
	for i := 0; i < s.size; i++ {
		s.jobs[i] = nil
	}
	s.size = 0
}

func (s *Scheduler) removeByCondition(shouldRemove func(*Job) bool) {
	i := 0

	for {
		found := false

		for ; i < s.size; i++ {
			if shouldRemove(s.jobs[i]) {
				found = true
				break
			}
		}
		if !found {
			return
		}

		for j := i + 1; j < s.size; j++ {
			s.jobs[i] = s.jobs[j]
			i++
		}
		s.size--
		s.jobs[s.size] = nil
	}
}

func Every(interval uint64) *Job {
	return defaultScheduler.Every(interval)
}

func Start() chan bool {
	return defaultScheduler.Start()
}

func Remove(j interface{}) {
	defaultScheduler.Remove(j)
}

func Scheduled(j interface{}) bool {
	for _, job := range defaultScheduler.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

func NextRun() (job *Job, time time.Time) {
	return defaultScheduler.NextRun()
}
