package orchestration

type GroomingJob struct {
	BeadID   string
	RepoPath string
	Prompt   string
}

type GroomingWorker struct {
	jobs chan GroomingJob
}

func NewGroomingWorker(buffer int) *GroomingWorker {
	if buffer <= 0 {
		buffer = 100
	}
	return &GroomingWorker{
		jobs: make(chan GroomingJob, buffer),
	}
}

func (w *GroomingWorker) Enqueue(job GroomingJob) {
	w.jobs <- job
}

func (w *GroomingWorker) Jobs() <-chan GroomingJob {
	return w.jobs
}
