package interop

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/complyue/hbi/pkg/he"
)

// ExposeInterOpValues exposes some common names in other languages for interop with them
func ExposeInterOpValues(he *he.HostingEnv) {
	ve := he.AnkoEnv()
	ve.ExecuteContext(context.Background(), `
None = nil
True = true
False = false
null = nil
`)
	ve.Define("NaN", math.NaN())
	ve.Define("nan", math.NaN())
}

// JSONObj converts a map into the json representation as a string
func JSONObj(o map[string]interface{}) string {
	encoded, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// JSONArray converts a slice into the json representation as a string
func JSONArray(a []interface{}) string {
	encoded, err := json.Marshal(a)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// JSONStr converts a string into the json representation as a string
func JSONStr(v interface{}) string {
	s := fmt.Sprintf("%s", v)
	encoded, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
