package prompt

type RefinementJob struct {
	BeatID          string
	RepoPath        string
	ExcludeAgentIDs []string
}

type RefinementQueue struct {
	jobs chan RefinementJob
}

func NewRefinementQueue(buffer int) *RefinementQueue {
	if buffer <= 0 {
		buffer = 100
	}
	return &RefinementQueue{
		jobs: make(chan RefinementJob, buffer),
	}
}

func (q *RefinementQueue) Enqueue(job RefinementJob) {
	q.jobs <- job
}

func (q *RefinementQueue) Jobs() <-chan RefinementJob {
	return q.jobs
}