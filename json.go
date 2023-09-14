package main

import "encoding/json"

func UnmarshalJSON[T any](data []byte) T {
	var parsed T
	json.Unmarshal(data, &parsed)
	return parsed
}
