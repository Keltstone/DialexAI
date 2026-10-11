package coder

import (
	"artix/internal/forgesec"
	"artix/pkg/policy"
)

func mintVerifiedApprovalForTest(approver, author, state, commitSHA, source string) *policy.PRApproval {
	sig := forgesec.SignToken(approver, author, state, commitSHA, source)
	return &policy.PRApproval{
		ApproverUsername: approver,
		AuthorUsername:   author,
		State:            state,
		CommitSHA:        commitSHA,
		Signature:        sig,
		Source:           source,
		VerifiedByForge:  true,
	}
}
