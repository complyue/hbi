package interop

import (
	"math"

	"github.com/complyue/hbi/pkg/he"
)

// ExposeInterOpValues exposes some common names in other languages for interop with them
func ExposeInterOpValues(he *he.HostingEnv) {
	he.ExposeValue("None", nil)
	he.ExposeValue("True", true)
	he.ExposeValue("False", false)
	he.ExposeValue("null", nil)
	he.ExposeValue("NaN", math.NaN())
	he.ExposeValue("nan", math.NaN())
}
