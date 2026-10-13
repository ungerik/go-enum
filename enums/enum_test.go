package enums

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnum_KindMethodsWithoutFind(t *testing.T) {
	// Enum is exported with exported fields, so code importing the package
	// can create an Enum without Find, and its kind methods must use the
	// predeclared underlying type instead of panicking
	for underlying, want := range map[string][3]any{
		"string":  {true, false, "string"},
		"byte":    {false, true, "number"},
		"float64": {false, false, "number"},
		"bool":    {false, false, "boolean"},
	} {
		e := &Enum{Type: "Status", Underlying: underlying}
		assert.Equal(t, want, [3]any{e.IsStringType(), e.IsIntType(), e.JSONType()}, underlying)
	}
}
