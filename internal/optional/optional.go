// Package optional provides versions of primitive types that can
// be nil. These are useful in methods that update some of an API object's
// fields.
package optional

import (
	"fmt"
	"strings"
)

type (
	// Bool is either a bool or nil.
	Bool interface{}

	// String is either a string or nil.
	String interface{}

	// Int is either an int or nil.
	Int interface{}

	// Uint is either a uint or nil.
	Uint interface{}

	// Float64 is either a float64 or nil.
	Float64 interface{}
)

// ToBool returns its argument as a bool.
// It panics if its argument is nil or not a bool.
func ToBool(v Bool) bool {
	x, ok := v.(bool)
	if !ok {
		doPanic("Bool", v)
	}
	return x
}

// ToString returns its argument as a string.
// It panics if its argument is nil or not a string.
func ToString(v String) string {
	x, ok := v.(string)
	if !ok {
		doPanic("String", v)
	}
	return x
}

// ToInt returns its argument as an int.
// It panics if its argument is nil or not an int.
func ToInt(v Int) int {
	x, ok := v.(int)
	if !ok {
		doPanic("Int", v)
	}
	return x
}

// ToUint returns its argument as a uint.
// It panics if its argument is nil or not a uint.
func ToUint(v Uint) uint {
	x, ok := v.(uint)
	if !ok {
		doPanic("Uint", v)
	}
	return x
}

// ToFloat64 returns its argument as a float64.
// It panics if its argument is nil or not a float64.
func ToFloat64(v Float64) float64 {
	x, ok := v.(float64)
	if !ok {
		doPanic("Float64", v)
	}
	return x
}

func doPanic(capType string, v interface{}) {
	panic(fmt.Sprintf("optional.%s value should be %s, got %T", capType, strings.ToLower(capType), v))
}
