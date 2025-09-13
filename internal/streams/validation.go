package streams

import "github.com/smazurov/videonode/internal/types"

// ValidationStorage provides access to validation data through a repository
type ValidationStorage struct {
	repository ValidationRepository
}

// NewValidationStorage creates a new validation storage
func NewValidationStorage(repo ValidationRepository) *ValidationStorage {
	return &ValidationStorage{
		repository: repo,
	}
}

// GetValidation returns the current validation data
func (v *ValidationStorage) GetValidation() *types.ValidationResults {
	return v.repository.GetValidation()
}

// UpdateValidation updates the validation data
func (v *ValidationStorage) UpdateValidation(results *types.ValidationResults) error {
	return v.repository.UpdateValidation(results)
}

// Save is an alias for UpdateValidation to implement the Storage interface
func (v *ValidationStorage) Save(validation *types.ValidationResults) error {
	return v.UpdateValidation(validation)
}

// Load retrieves validation data
func (v *ValidationStorage) Load() (*types.ValidationResults, error) {
	validation := v.GetValidation()
	if validation == nil {
		// Return empty results instead of error for missing data
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
