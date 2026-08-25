package llmcatalog

import (
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
)

// ErrInvalid is the refusal a caller gets for input the catalog will not take.
// Detail carries what was wrong; InvalidField says which field it was about.
var ErrInvalid = apierr.New(apierr.KindInvalid, "invalid model input")

// InvalidField is the input field a refusal is about, in the catalog's own
// vocabulary rather than any edge's.
//
// An edge points at whatever it calls that field -- a shell flag, a JSON key --
// and this service knows neither. Saying "--name" here would put a flag that
// only one caller has into an error every caller can receive.
type InvalidField struct {
	// Field is the input field, or empty when the refusal is not about one.
	Field string
	// Message reads as the rest of a sentence beginning with the field.
	Message string
	err     error
}

func (e *InvalidField) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + " " + e.Message
}

// Unwrap reports ErrInvalid, so errors.Is matches it and a transport maps the
// Kind without knowing this type.
func (e *InvalidField) Unwrap() error { return e.err }

func invalidf(field, format string, args ...any) *InvalidField {
	return &InvalidField{Field: field, Message: fmt.Sprintf(format, args...), err: ErrInvalid}
}
