package policy

import "artix/internal/forgesec"

// MintVerifiedForgeApprovalForTest creates an approval for unit and integration testing.
// This function resides strictly in test code and is NOT exported in production binaries.
func MintVerifiedForgeApprovalForTest(approver, author, state, commitSHA, source string) *PRApproval {
	sig := forgesec.SignToken(approver, author, state, commitSHA, source)
	return &PRApproval{
		ApproverUsername: approver,
		AuthorUsername:   author,
		State:            state,
		CommitSHA:        commitSHA,
		Signature:        sig,
		Source:           source,
		VerifiedByForge:  true,
	}
}
