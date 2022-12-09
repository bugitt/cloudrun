package v1alpha1

type Status string

const (
	// StatusPending means the CRD is pending to be deployed.
	StatusPending Status = "Pending"
	// StatusDoing means the CRD is being deployed.
	StatusDoing Status = "Doing"
	// StatusDone means the CRD has been deployed.
	StatusDone Status = "Done"
	// StatusFailed means the CRD failed to be deployed.
	StatusFailed Status = "Failed"
	// StatusUnknown means the CRD status is unknown.
	StatusUnknown Status = "Unknown"
)

type StatusWithMessage struct {
	Status Status `json:"status"`
	// Message is mainly used to store the error message when the CRD is failed.
	Message string `json:"message,omitempty"`
}
