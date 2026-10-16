package selfdev

// CandidateState is the lifecycle of one candidate build. Only the controller
// advances it, from verifiable facts; model output never sets a state.
type CandidateState string

const (
	CandidatePrepared    CandidateState = "prepared"
	CandidateVerifying   CandidateState = "verifying"
	CandidateVerified    CandidateState = "verified"
	CandidateFailed      CandidateState = "failed"
	CandidateCancelled   CandidateState = "cancelled"
	CandidateNeedsReview CandidateState = "needs_review"
)

// ActivationState is the lifecycle of one switch to a verified candidate.
// Only the controller advances it; a candidate cannot report itself ready.
type ActivationState string

const (
	ActivationPending          ActivationState = "pending"
	ActivationStarting         ActivationState = "starting"
	ActivationReady            ActivationState = "ready"
	ActivationActive           ActivationState = "active"
	ActivationRollingBack      ActivationState = "rolling_back"
	ActivationRolledBack       ActivationState = "rolled_back"
	ActivationRecoveryRequired ActivationState = "recovery_required"
)
