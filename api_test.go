package jmespath

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jmespath/go-jmespath/internal/testify/assert"
)

func TestValidUncompiledExpressionSearches(t *testing.T) {
	assert := assert.New(t)
	var j = []byte(`{"foo": {"bar": {"baz": [0, 1, 2, 3, 4]}}}`)
	var d interface{}
	err := json.Unmarshal(j, &d)
	assert.Nil(err)
	result, err := Search("foo.bar.baz[2]", d)
	assert.Nil(err)
	assert.Equal(2.0, result)
}

func TestValidPrecompiledExpressionSearches(t *testing.T) {
	assert := assert.New(t)
	data := make(map[string]interface{})
	data["foo"] = "bar"
	precompiled, err := Compile("foo")
	assert.Nil(err)
	result, err := precompiled.Search(data)
	assert.Nil(err)
	assert.Equal("bar", result)
}

func TestInvalidPrecompileErrors(t *testing.T) {
	assert := assert.New(t)
	_, err := Compile("not a valid expression")
	assert.NotNil(err)
}

func TestInvalidMustCompilePanics(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r)
	}()
	MustCompile("not a valid expression")
}

func TestToEntries(t *testing.T) {
	assert := assert.New(t)
	data := make(map[string]interface{})
	data["foo"] = "bar"
	data["baz"] = 42
	result, err := Search("to_entries(@)", data)
	assert.Nil(err)

	entries, ok := result.([]interface{})
	assert.True(ok)
	assert.Equal(2, len(entries))

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		assert.True(ok)
		_, hasKey := entryMap["key"]
		_, hasValue := entryMap["value"]
		assert.True(hasKey)
		assert.True(hasValue)
	}
}

func TestJSONNumberIsTreatedAsNumber(t *testing.T) {
	assert := assert.New(t)
	data := map[string]interface{}{
		"total": json.Number("300"),
		"items": []interface{}{
			map[string]interface{}{"n": json.Number("1")},
			map[string]interface{}{"n": json.Number("3")},
			map[string]interface{}{"n": json.Number("2")},
		},
	}

	result, err := Search("total", data)
	assert.Nil(err)
	assert.Equal(json.Number("300"), result)

	result, err = Search("total > `0`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("total == `300`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("type(total)", data)
	assert.Nil(err)
	assert.Equal("number", result)

	result, err = Search("ceil(total)", data)
	assert.Nil(err)
	assert.Equal(300.0, result)

	result, err = Search("abs(total)", data)
	assert.Nil(err)
	assert.Equal(300.0, result)

	result, err = Search("to_number(total)", data)
	assert.Nil(err)
	assert.Equal(300.0, result)

	result, err = Search("sum(items[*].n)", data)
	assert.Nil(err)
	assert.Equal(6.0, result)

	result, err = Search("max_by(items, &n).n", data)
	assert.Nil(err)
	assert.Equal(json.Number("3"), result)

	result, err = Search("sort_by(items, &n)[0].n", data)
	assert.Nil(err)
	assert.Equal(json.Number("1"), result)
}

func TestJSONNumberIdentityPreservesPrecision(t *testing.T) {
	assert := assert.New(t)
	big := json.Number("9007199254740993")
	smaller := json.Number("9007199254740992")
	data := map[string]interface{}{
		"id":      big,
		"smaller": smaller,
		"items": []interface{}{
			map[string]interface{}{"n": big},
			map[string]interface{}{"n": smaller},
		},
	}
	result, err := Search("id", data)
	assert.Nil(err)
	assert.Equal(big, result)

	result, err = Search("id > smaller", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("id == `9007199254740993`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("id != `9007199254740992`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("`9007199254740993` != `9007199254740992`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("max_by(items, &n).n", data)
	assert.Nil(err)
	assert.Equal(big, result)

	result, err = Search("sort_by(items, &n)[0].n", data)
	assert.Nil(err)
	assert.Equal(smaller, result)
}

func TestJSONLiteralResultKeepsFloat64Compatibility(t *testing.T) {
	assert := assert.New(t)
	expression := MustCompile("`[1, 2, 3]`")
	for i := 0; i < 2; i++ {
		result, err := expression.Search(nil)
		assert.Nil(err)
		assert.Equal([]interface{}{1.0, 2.0, 3.0}, result)
	}

	result, err := Search("`9007199254740993`", nil)
	assert.Nil(err)
	assert.Equal(json.Number("9007199254740993"), result)
}

func TestJSONNumberFromUseNumberDecoder(t *testing.T) {
	assert := assert.New(t)
	raw := []byte(`{"result":{"total":300}}`)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data interface{}
	assert.Nil(decoder.Decode(&data))

	result, err := Search("result.total", data)
	assert.Nil(err)
	assert.Equal(json.Number("300"), result)

	result, err = Search("result.total > `0`", data)
	assert.Nil(err)
	assert.Equal(true, result)

	result, err = Search("type(result.total)", data)
	assert.Nil(err)
	assert.Equal("number", result)

	result, err = Search("ceil(result.total)", data)
	assert.Nil(err)
	assert.Equal(300.0, result)
}
