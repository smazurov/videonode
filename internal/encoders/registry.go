package encoders

import "github.com/smazurov/videonode/internal/encoders/validation"

// CreateValidatorRegistry creates and populates the validator registry (shared between files)
func CreateValidatorRegistry() *validation.ValidatorRegistry {
	registry := validation.NewValidatorRegistry()

	// Register all validators
	registry.Register(validation.NewVaapiValidator())
	registry.Register(validation.NewNvencValidator())
	registry.Register(validation.NewAmfValidator())
	registry.Register(validation.NewQsvValidator())
	registry.Register(validation.NewVideoToolboxValidator())
	registry.Register(validation.NewRkmppValidator())
	registry.Register(validation.NewV4l2m2mValidator())
	registry.Register(validation.NewGenericValidator()) // Fallback validator last

	return registry
}
