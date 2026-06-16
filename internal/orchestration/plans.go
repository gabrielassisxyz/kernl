package orchestration

type Plan struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Steps  []PlanStep `json:"steps"`
	Status string     `json:"status"`
}

type PlanStep struct {
	BeadID    string `json:"beadId"`
	Action    string `json:"action"`
	State     string `json:"state"`
	Completed bool   `json:"completed"`
}
