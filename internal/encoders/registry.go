package encoders

import "github.com/smazurov/videonode/internal/encoders/validation"

// CreateValidatorRegistry creates and populates the validator registry (shared between files)
func CreateValidatorRegistry() *validation.ValidatorRegistry {
	registry := validation.NewValidatorRegistry()

	// Register validators in priority order
	registry.Register(validation.NewRkmppValidator())
	registry.Register(validation.NewVaapiValidator())
	registry.Register(validation.NewGenericValidator()) // Fallback validator last

	return registry
}
