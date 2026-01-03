package types

// ValidationProvider interface for accessing validation data.
type ValidationProvider interface {
	GetValidation() *ValidationResults
	UpdateValidation(validation *ValidationResults) error
}
