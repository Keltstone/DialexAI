package main

import (
	"fmt"
	"io"

	"artix/pkg/audit"
	"artix/pkg/coder"
)

// ExplainResult encapsulates root-cause and reproduction details for a task.
type ExplainResult struct {
	TaskID           string             `json:"taskId"`
	Status           string             `json:"status"`
	Rounds           int                `json:"rounds"`
	FailingPhase     string             `json:"failingPhase,omitempty"`
	RejectionReasons []string           `json:"rejectionReasons,omitempty"`
	RoundsDetail     []coder.RoundTrace `json:"roundsDetail"`
	ReproduceCommand string             `json:"reproduceCommand,omitempty"`
}

func runExplain(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Error: task-id argument is required for artix explain\n")
		return 1
	}

	taskID := args[0]

	traces, err := coder.ReadRoundTraces(cwd, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading round traces: %v\n", err)
		return 1
	}

	// Read audit log to get final status if available
	finalStatus := "UNKNOWN"
	events, _ := audit.Default(cwd).ReadEvents()
	for _, e := range events {
		if e.StorySpecID == taskID {
			finalStatus = string(e.Status)
		}
	}

	failingPhase := ""
	var rejectionReasons []string
	reproduceCmd := ""

	for _, tr := range traces {
		if tr.ReviewerVerdict == "FAILED" || tr.ReviewerVerdict == "TIMEOUT" || tr.ReviewerVerdict == "OFFLINE_CACHE_MISS" || tr.ReviewerVerdict == "REJECTED" {
			if tr.Phase != "" {
				failingPhase = tr.Phase
			}
			rejectionReasons = append(rejectionReasons, tr.BlockingIssues...)
			reproduceCmd = fmt.Sprintf("artix replay %s --round %d", taskID, tr.Round)
		}
	}

	if failingPhase == "" && len(traces) > 0 {
		failingPhase = traces[len(traces)-1].Phase
		if failingPhase == "" {
			failingPhase = "CONVERGENCE"
		}
	}

	if reproduceCmd == "" && len(traces) > 0 {
		reproduceCmd = fmt.Sprintf("artix replay %s --round %d", taskID, traces[len(traces)-1].Round)
	}

	if finalStatus == "UNKNOWN" && len(traces) > 0 {
		finalStatus = traces[len(traces)-1].ReviewerVerdict
	}

	res := ExplainResult{
		TaskID:           taskID,
		Status:           finalStatus,
		Rounds:           len(traces),
		FailingPhase:     failingPhase,
		RejectionReasons: rejectionReasons,
		RoundsDetail:     traces,
		ReproduceCommand: reproduceCmd,
	}

	if isJSON {
		sendJSON(res)
		return 0
	}

	fmt.Fprintf(human, "Task ID: %s\n", res.TaskID)
	fmt.Fprintf(human, "Final Status: %s\n", res.Status)
	fmt.Fprintf(human, "Rounds Run: %d\n", res.Rounds)
	if res.FailingPhase != "" {
		fmt.Fprintf(human, "Failing Phase: %s\n", res.FailingPhase)
	}
	if len(res.RejectionReasons) > 0 {
		fmt.Fprintf(human, "Rejection Reasons:\n")
		for _, r := range res.RejectionReasons {
			fmt.Fprintf(human, "  - %s\n", r)
		}
	}
	if res.ReproduceCommand != "" {
		fmt.Fprintf(human, "Reproduce Locally:\n  %s\n", res.ReproduceCommand)
	}

	return 0
}
