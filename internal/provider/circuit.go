package provider

import "time"

func CircuitCooldown(steps []int, failureCount int) time.Duration {
	if failureCount <= 0 || len(steps) == 0 {
		return 0
	}
	index := failureCount - 1
	if index >= len(steps) {
		index = len(steps) - 1
	}
	seconds := steps[index]
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
