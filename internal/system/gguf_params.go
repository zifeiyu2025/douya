package system

import (
	"regexp"
	"strconv"
)

var sizeLabelRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)x(\d+(?:\.\d+)?)\s*([kmgtq]?)b$|^(\d+(?:\.\d+)?)\s*([kmgtq]?)b$`)

func EstimateNParamsFromSizeLabel(sizeLabel string) int64 {
	if sizeLabel == "" {
		return 0
	}
	matches := sizeLabelRe.FindStringSubmatch(sizeLabel)
	if matches == nil {
		return 0
	}
	if matches[1] != "" {
		expertCount, _ := strconv.ParseFloat(matches[1], 64)
		perExpert, _ := strconv.ParseFloat(matches[2], 64)
		scale := parseScale(matches[3])
		return int64(expertCount * perExpert * scale)
	}
	value, _ := strconv.ParseFloat(matches[4], 64)
	scale := parseScale(matches[5])
	return int64(value * scale)
}

func parseScale(s string) float64 {
	switch s {
	case "k", "K":
		return 1e3
	case "m", "M":
		return 1e6
	case "g", "G":
		return 1e9
	case "t", "T":
		return 1e12
	case "q", "Q":
		return 1e15
	default:
		return 1e9
	}
}

func ResolveNParams(serverNParams float64, ggufMeta *GGUFMetadata) float64 {
	if serverNParams > 0 {
		return serverNParams
	}
	if ggufMeta == nil {
		return 0
	}
	if ggufMeta.NParams > 0 {
		return float64(ggufMeta.NParams)
	}
	estimated := EstimateNParamsFromSizeLabel(ggufMeta.SizeLabel)
	if estimated > 0 {
		return float64(estimated)
	}
	return 0
}
