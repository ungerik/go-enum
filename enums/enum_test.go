package enums

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnum_KindMethodsWithoutFind(t *testing.T) {
	// Enum is exported with exported fields, so code importing the package
	// can create an Enum without Find, and its kind methods must use the
	// predeclared underlying type instead of panicking
	//
	// Value: protects=an Enum created without Find with an underlying type that is not predeclared, like a package type, has kind methods that don't panic;
	// fails_when=basicType returns nil or a guessed kind for an unknown underlying type, so the kind methods panic or claim a string or integer type;
	// why_new=the rows above only use predeclared types; coverage shows the invalid fallback of basicType never runs;
	// seam=none;
	// no_claim=does not generate methods for such an Enum, only checks its kind methods
	for underlying, want := range map[string][3]any{
		"string":  {true, false, "string"},
		"byte":    {false, true, "number"},
		"float64": {false, false, "number"},
		"bool":    {false, false, "boolean"},
		// Without Find the underlying type of a package type is unknown,
		// so it is neither string nor integer and String() is not generated
		"MyString": {false, false, "string"},
	} {
		e := &Enum{Type: "Status", Underlying: underlying}
		assert.Equal(t, want, [3]any{e.IsStringType(), e.IsIntType(), e.JSONType()}, underlying)
	}
}
