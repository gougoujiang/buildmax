package user

// Register validates the request fields and returns an error if any are invalid.
// Validation rules:
//   - Name must not be empty
//   - Email must contain both "@" and "." and must not start with "@"
//   - Password must be at least 8 characters long
func Register(req RegisterRequest) error {
	// TODO: add validation — return the appropriate sentinel error for each rule
	return nil
}
