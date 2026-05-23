package policy

// Decision is the output of the policy engine.
type Decision struct {
	Verdict           Verdict    `json:"verdict"`
	Reason            string     `json:"reason,omitempty"`
	RuleName          string     `json:"rule_name,omitempty"`
	RequiredApprovers []Approver `json:"required_approvers,omitempty"`
}
