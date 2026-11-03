package types

// QuotaRejection is created only at a local, authoritative quota check. It is
// never inferred from an upstream status/code or serialized to the client.
type QuotaRejection struct {
	Source     string
	ResourceID int
	Available  int64
	Required   int64
	Known      bool
	Message    string
	Cause      error
	// Reason is set only by an authoritative local check (e.g. wallet funds
	// temporarily reserved by another request). Empty derives from the numbers.
	Reason string
}

const (
	QuotaReasonExhausted       = "exhausted"
	QuotaReasonRequestTooLarge = "request_exceeds_available"
	QuotaReasonInFlight        = "in_flight"
	QuotaReasonUnknown         = "unknown"
)

func (e *QuotaRejection) RejectionReason() string {
	if e == nil {
		return QuotaReasonUnknown
	}
	if e.Reason != "" {
		return e.Reason
	}
	if !e.Known {
		return QuotaReasonUnknown
	}
	if e.Available <= 0 {
		return QuotaReasonExhausted
	}
	if e.Required > e.Available {
		return QuotaReasonRequestTooLarge
	}
	return QuotaReasonUnknown
}

func (e *QuotaRejection) Error() string { return e.Message }

func (e *QuotaRejection) Unwrap() error { return e.Cause }
