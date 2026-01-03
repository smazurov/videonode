package led

// Controller abstracts LED hardware control across different SBC boards.
// Implementations handle board-specific LED naming and capabilities.
type Controller interface {
	// Set controls an LED's state and optional pattern
	// Parameters:
	//   ledType: board-specific LED identifier (e.g., "user", "system", "blue")
	//   enabled: whether the LED should be on or off
	//   pattern: optional blinking pattern (e.g., "solid", "blink", "heartbeat")
	//            empty string means no pattern change
	Set(ledType string, enabled bool, pattern string) error

	// Available returns the list of LED types supported by this controller
	Available() []string

	// Patterns returns the list of patterns supported by this controller
	Patterns() []string
}
