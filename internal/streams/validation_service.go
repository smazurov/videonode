package streams

import (
	"github.com/smazurov/videonode/internal/types"
)

// ValidationService handles encoder validation data operations.
type ValidationService struct {
	store Store
}

// NewValidationService creates a new validation service.
func NewValidationService(store Store) *ValidationService {
	// Load existing validation data, ignore errors as empty results are acceptable
	_ = store.Load()

	return &ValidationService{
		store: store,
	}
}

// GetValidation returns the current validation data.
func (v *ValidationService) GetValidation() *types.ValidationResults {
	return v.store.GetValidation()
}

// UpdateValidation updates the validation data.
func (v *ValidationService) UpdateValidation(results *types.ValidationResults) error {
	return v.store.UpdateValidation(results)
}

// Load retrieves validation data, returning empty results if none exist.
func (v *ValidationService) Load() (*types.ValidationResults, error) {
	validation := v.GetValidation()
	if validation == nil {
		return &types.ValidationResults{
			H264: types.CodecValidation{
				Working: []string{},
				Failed:  []string{},
			},
			H265: types.CodecValidation{
				Working: []string{},
				Failed:  []string{},
			},
		}, nil
	}
	return validation, nil
}

// Save is an alias for UpdateValidation.
func (v *ValidationService) Save(validation *types.ValidationResults) error {
	return v.UpdateValidation(validation)
}
