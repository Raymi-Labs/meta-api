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
}

func (e *QuotaRejection) Error() string { return e.Message }

func (e *QuotaRejection) Unwrap() error { return e.Cause }
