package alicloud

import (
	"encoding/json"
	"strings"
)

// Convert the result for an array and returns a Json string
func convertListStringToJsonString(configured []string) string {
	if len(configured) < 1 {
		return ""
	}
	result := "["
	for i, v := range configured {
		if v == "" {
			continue
		}
		result += "\"" + v + "\""
		if i < len(configured)-1 {
			result += ","
		}
	}
	result += "]"
	return result
}

func convertJsonStringToListString(configured string) ([]string, error) {
	result := make([]string, 0)
	if err := json.Unmarshal([]byte(configured), &result); err != nil {
		return nil, err
	}

	return result, nil
}

func trimStringQuotes(input string) string {
	return strings.TrimPrefix(strings.TrimSuffix(input, "\""), "\"")
}
