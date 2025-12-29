package jmespath

import (
	"testing"

	"github.com/jmespath/go-jmespath"
)

func FuzzParse(f *testing.F) {
	f.Fuzz(func(t *testing.T, data string) {
		p := jmespath.NewParser()
		_, _ = p.Parse(data)
	})
}
