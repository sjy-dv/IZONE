package k8s

import (
	"fmt"
	"math"
)

func phaseEqual(cur, pre string) string {
	if cur == pre {
		return "Stable"
	}
	return fmt.Sprintf("Changed in %s", pre)
}

func percentage(cur, pre int64) string {
	// avoid error panic: runtime error: integer divide by zero
	if cur == 0 {
		cur = 1
	}
	if pre == 0 {
		pre = 1
	}
	per := ((cur - pre) / pre) * 100
	if cur < pre {
		return fmt.Sprintf("%f%% decrease", math.Abs(float64(per)))
	}
	return fmt.Sprintf("%d%% increase", per)
}
