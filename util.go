package jmespath

import (
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strconv"
)

// literalNumber preserves the exact spelling of a number in a JMESPath JSON
// literal while it is being evaluated. Search converts it back to float64 at
// the API boundary to preserve the package's existing result types.
type literalNumber string

func (n literalNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.Number(n))
}

// IsFalse determines if an object is false based on the JMESPath spec.
// JMESPath defines false values to be any of:
// - An empty string array, or hash.
// - The boolean value false.
// - nil
func isFalse(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return !v
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case string:
		return len(v) == 0
	case nil:
		return true
	}
	// Try the reflection cases before returning false.
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Struct:
		// A struct type will never be false, even if
		// all of its values are the zero type.
		return false
	case reflect.Slice, reflect.Map:
		return rv.Len() == 0
	case reflect.Ptr:
		if rv.IsNil() {
			return true
		}
		// If it's a pointer type, we'll try to deref the pointer
		// and evaluate the pointer value for isFalse.
		element := rv.Elem()
		return isFalse(element.Interface())
	}
	return false
}

// ObjsEqual is a generic object equality check.
// It will take two arbitrary objects and recursively determine
// if they are equal.
func objsEqual(left interface{}, right interface{}) bool {
	if cmp, ok := compareNumbers(left, right); ok {
		return cmp == 0
	}
	switch leftValue := left.(type) {
	case []interface{}:
		rightValue, ok := right.([]interface{})
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for i := range leftValue {
			if !objsEqual(leftValue[i], rightValue[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		rightValue, ok := right.(map[string]interface{})
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			rightItem, ok := rightValue[key]
			if !ok || !objsEqual(value, rightItem) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(left, right)
}

// compareNumbers compares JSON numbers exactly. Converting json.Number to
// float64 here would collapse distinct integers above 2^53.
func compareNumbers(left interface{}, right interface{}) (int, bool) {
	// A float64 input has already adopted IEEE-754 semantics. Compare mixed
	// float64 values the same way the original implementation did so ordinary
	// decimals such as 1.2 remain compatible.
	if _, ok := left.(float64); ok {
		return compareFloatNumbers(left, right)
	}
	if _, ok := right.(float64); ok {
		return compareFloatNumbers(left, right)
	}
	leftNum, ok := toExactNum(left)
	if !ok {
		return 0, false
	}
	rightNum, ok := toExactNum(right)
	if !ok {
		return 0, false
	}
	return leftNum.Cmp(rightNum), true
}

func compareFloatNumbers(left interface{}, right interface{}) (int, bool) {
	leftNum, ok := toNum(left)
	if !ok {
		return 0, false
	}
	rightNum, ok := toNum(right)
	if !ok {
		return 0, false
	}
	switch {
	case leftNum < rightNum:
		return -1, true
	case leftNum > rightNum:
		return 1, true
	case leftNum == rightNum:
		return 0, true
	default:
		// NaN is unordered, so neither operand can establish equality.
		return 0, false
	}
}

func toExactNum(data interface{}) (*big.Rat, bool) {
	switch v := data.(type) {
	case float64:
		n := new(big.Rat).SetFloat64(v)
		return n, n != nil
	case json.Number:
		n, ok := new(big.Rat).SetString(v.String())
		return n, ok
	case literalNumber:
		n, ok := new(big.Rat).SetString(string(v))
		return n, ok
	}
	return nil, false
}

// toNum converts a JSON number into float64.
// encoding/json.Decoder.UseNumber() stores numbers as json.Number instead of
// float64; arithmetic functions still operate on float64 for compatibility.
func toNum(data interface{}) (float64, bool) {
	switch v := data.(type) {
	case float64:
		return v, true
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	case literalNumber:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func wrapLiteralNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case json.Number:
		return literalNumber(v.String())
	case []interface{}:
		for i := range v {
			v[i] = wrapLiteralNumbers(v[i])
		}
	case map[string]interface{}:
		for key := range v {
			v[key] = wrapLiteralNumbers(v[key])
		}
	}
	return value
}

func unwrapLiteralNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case literalNumber:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return json.Number(v)
		}
		exact, exactOK := new(big.Rat).SetString(string(v))
		rounded := new(big.Rat).SetFloat64(n)
		if exactOK && exact.IsInt() && (rounded == nil || exact.Cmp(rounded) != 0) {
			return json.Number(v)
		}
		return n
	case []interface{}:
		result := make([]interface{}, len(v))
		for i := range v {
			result[i] = unwrapLiteralNumbers(v[i])
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key := range v {
			result[key] = unwrapLiteralNumbers(v[key])
		}
		return result
	}
	return value
}

// SliceParam refers to a single part of a slice.
// A slice consists of a start, a stop, and a step, similar to
// python slices.
type sliceParam struct {
	N         int
	Specified bool
}

// Slice supports [start:stop:step] style slicing that's supported in JMESPath.
func slice(slice []interface{}, parts []sliceParam) ([]interface{}, error) {
	computed, err := computeSliceParams(len(slice), parts)
	if err != nil {
		return nil, err
	}
	start, stop, step := computed[0], computed[1], computed[2]
	result := []interface{}{}
	if step > 0 {
		for i := start; i < stop; i += step {
			result = append(result, slice[i])
		}
	} else {
		for i := start; i > stop; i += step {
			result = append(result, slice[i])
		}
	}
	return result, nil
}

func computeSliceParams(length int, parts []sliceParam) ([]int, error) {
	var start, stop, step int
	if !parts[2].Specified {
		step = 1
	} else if parts[2].N == 0 {
		return nil, errors.New("Invalid slice, step cannot be 0")
	} else {
		step = parts[2].N
	}
	var stepValueNegative bool
	if step < 0 {
		stepValueNegative = true
	} else {
		stepValueNegative = false
	}

	if !parts[0].Specified {
		if stepValueNegative {
			start = length - 1
		} else {
			start = 0
		}
	} else {
		start = capSlice(length, parts[0].N, step)
	}

	if !parts[1].Specified {
		if stepValueNegative {
			stop = -1
		} else {
			stop = length
		}
	} else {
		stop = capSlice(length, parts[1].N, step)
	}
	return []int{start, stop, step}, nil
}

func capSlice(length int, actual int, step int) int {
	if actual < 0 {
		actual += length
		if actual < 0 {
			if step < 0 {
				actual = -1
			} else {
				actual = 0
			}
		}
	} else if actual >= length {
		if step < 0 {
			actual = length - 1
		} else {
			actual = length
		}
	}
	return actual
}

// ToArrayNum converts an empty interface type to a slice of float64.
// If any element in the array cannot be converted, then nil is returned
// along with a second value of false.
func toArrayNum(data interface{}) ([]float64, bool) {
	// Is there a better way to do this with reflect?
	if d, ok := data.([]interface{}); ok {
		result := make([]float64, len(d))
		for i, el := range d {
			item, ok := toNum(el)
			if !ok {
				return nil, false
			}
			result[i] = item
		}
		return result, true
	}
	return nil, false
}

// toArrayExactNum validates a number array without converting its elements.
// This allows comparison functions to preserve json.Number precision.
func toArrayExactNum(data interface{}) ([]interface{}, bool) {
	d, ok := data.([]interface{})
	if !ok {
		return nil, false
	}
	if len(d) == 0 {
		return []interface{}{}, true
	}
	for _, el := range d {
		if _, ok := toExactNum(el); !ok {
			return nil, false
		}
	}
	return d, true
}

// ToArrayStr converts an empty interface type to a slice of strings.
// If any element in the array cannot be converted, then nil is returned
// along with a second value of false.  If the input data could be entirely
// converted, then the converted data, along with a second value of true,
// will be returned.
func toArrayStr(data interface{}) ([]string, bool) {
	// Is there a better way to do this with reflect?
	if d, ok := data.([]interface{}); ok {
		result := make([]string, len(d))
		for i, el := range d {
			item, ok := el.(string)
			if !ok {
				return nil, false
			}
			result[i] = item
		}
		return result, true
	}
	return nil, false
}

func isSliceType(v interface{}) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}
