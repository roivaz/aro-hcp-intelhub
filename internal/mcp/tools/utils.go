package tools

import (
	"encoding/json"
	"fmt"
)

func parseIntArgument(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("pr_number must be positive")
		}
		return int(v), nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("pr_number must be positive")
		}
		return v, nil
	default:
		return 0, fmt.Errorf("pr_number must be provided")
	}
}

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
