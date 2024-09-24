package validator

// Validator is any type capable to validate and having Validate method attached.
type Validator interface {
	Validate() error
}

// Validate validates type v.
func Validate(v Validator) error {
	return v.Validate()
}
