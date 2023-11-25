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
	per := ((cur - pre) / pre) * 100
	if per < 0 {
		return fmt.Sprintf("%f decrease", math.Abs(float64(per)))
	}
	return fmt.Sprintf("%d increase", per)
}
