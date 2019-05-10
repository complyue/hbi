package interop

import (
	"context"
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
