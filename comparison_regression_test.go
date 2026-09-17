package jmespath

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestEmptyAverageComparisonRegression(t *testing.T) {
	cases := []struct {
		expr string
		want interface{}
	}{
		{"avg(`[]`)", nil},
		{"avg(`[]`) == `100`", false},
		{"avg(`[]`) != `100`", true},
		{"avg(`[]`) == `null`", true},
		{"avg(`[]`) < `100`", nil},
		{"avg(`[]`) <= `100`", nil},
		{"avg(`[]`) > `100`", nil},
		{"avg(`[]`) >= `100`", nil},
		{"avg(`[100, 200]`)", float64(150)},
		{"[?avg(values) == `100`].id", []interface{}{"match"}},
	}
	data := []interface{}{
		map[string]interface{}{"id": "empty", "values": []interface{}{}},
		map[string]interface{}{"id": "match", "values": []interface{}{float64(100)}},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			for _, compiled := range []bool{false, true} {
				var got interface{}
				var err error
				if compiled {
					got, err = MustCompile(tc.expr).Search(data)
				} else {
					got, err = Search(tc.expr, data)
				}
				if err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("compiled=%v got=%#v want=%#v err=%v", compiled, got, tc.want, err)
				}
			}
		})
	}
}

func TestNaNComparisonRegression(t *testing.T) {
	nan := math.NaN()
	for _, other := range []interface{}{float64(100), json.Number("100"), literalNumber("100"), nan} {
		for _, pair := range [][2]interface{}{{nan, other}, {other, nan}} {
			if cmp, ok := compareNumbers(pair[0], pair[1]); ok {
				t.Fatalf("NaN comparison reported ordered: %d", cmp)
			}
			data := map[string]interface{}{"a": pair[0], "b": pair[1]}
			for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
				var want interface{}
				if op == "==" {
					want = false
				}
				if op == "!=" {
					want = true
				}
				got, err := Search("a "+op+" b", data)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("%v %s %v: got=%v want=%v err=%v", pair[0], op, pair[1], got, want, err)
				}
			}
		}
	}
}
