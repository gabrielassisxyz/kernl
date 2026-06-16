package backend

type LocalWorker struct{}

func NewLocalWorker() *LocalWorker {
	return &LocalWorker{}
}

func (w *LocalWorker) PreparePoll(repoPath string) error {
	return nil
}

func (w *LocalWorker) PrepareTake(repoPath string) error {
	return nil
}
