package pkg

import (
	"regexp"
	"strconv"
	"strings"
)

var versionTokenPattern = regexp.MustCompile(`\d+|[A-Za-z]+`)

func compareVersionTokens(a, b string) int {
	aTokens := versionTokenPattern.FindAllString(strings.ToLower(a), -1)
	bTokens := versionTokenPattern.FindAllString(strings.ToLower(b), -1)
	maxLen := len(aTokens)
	if len(bTokens) > maxLen {
		maxLen = len(bTokens)
	}
	for i := 0; i < maxLen; i++ {
		at := "0"
		bt := "0"
		if i < len(aTokens) {
			at = aTokens[i]
		}
		if i < len(bTokens) {
			bt = bTokens[i]
		}
		cmp := compareVersionToken(at, bt)
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareVersionToken(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	if aErr == nil && bErr == nil {
		if an > bn {
			return 1
		}
		if an < bn {
			return -1
		}
		return 0
	}
	if aErr == nil {
		return 1
	}
	if bErr == nil {
		return -1
	}
	aw := qualifierWeight(a)
	bw := qualifierWeight(b)
	if aw != bw {
		if aw > bw {
			return 1
		}
		return -1
	}
	return strings.Compare(a, b)
}

func qualifierWeight(q string) int {
	switch q {
	case "snapshot", "dev":
		return -5
	case "alpha", "a":
		return -4
	case "beta", "b":
		return -3
	case "pre", "preview":
		return -2
	case "rc":
		return -1
	case "release", "stable", "final":
		return 0
	default:
		return 0
	}
}
