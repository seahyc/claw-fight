package engines

import (
	"encoding/json"
	"fmt"
)

// ToInt converts any numeric-ish value to int.
func ToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// ToBool converts any value to bool.
func ToBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// GetInt reads an int from a map[string]any.
func GetInt(data map[string]any, key string) int {
	v, ok := data[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// GetString reads a string from a map[string]any.
func GetString(data map[string]any, key string) string {
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetStringSlice reads a []string from a map[string]any.
func GetStringSlice(data map[string]any, key string) []string {
	raw, ok := data[key]
	if !ok {
		return []string{}
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			result[i], _ = item.(string)
		}
		return result
	}
	return []string{}
}

// GetStringIntMap reads a map[string]int from a map[string]any.
func GetStringIntMap(data map[string]any, key string) map[string]int {
	raw, ok := data[key]
	if !ok {
		return map[string]int{}
	}
	switch v := raw.(type) {
	case map[string]int:
		return v
	case map[string]any:
		result := map[string]int{}
		for k, val := range v {
			switch n := val.(type) {
			case int:
				result[k] = n
			case float64:
				result[k] = int(n)
			}
		}
		return result
	}
	return map[string]int{}
}

// GetStringBoolMap reads a map[string]bool from a map[string]any.
func GetStringBoolMap(data map[string]any, key string) map[string]bool {
	raw, ok := data[key]
	if !ok {
		return map[string]bool{}
	}
	switch v := raw.(type) {
	case map[string]bool:
		return v
	case map[string]any:
		result := map[string]bool{}
		for k, val := range v {
			b, _ := val.(bool)
			result[k] = b
		}
		return result
	}
	return map[string]bool{}
}

// GetActionAmount extracts an "amount" int from Action.Data.
func GetActionAmount(action Action) (int, error) {
	amountRaw, ok := action.Data["amount"]
	if !ok {
		return 0, fmt.Errorf("amount is required")
	}
	switch v := amountRaw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case json.Number:
		n, err := v.Int64()
		return int(n), err
	default:
		return 0, fmt.Errorf("amount must be a number")
	}
}

// OtherPlayer returns the player that is NOT the given player.
func OtherPlayer(state *GameState, player PlayerID) PlayerID {
	if state.Players[0] == player {
		return state.Players[1]
	}
	return state.Players[0]
}
