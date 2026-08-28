package tools

import "encoding/json"

// MustJSON marshals value or panics.
//
// It is kept because the deleted alias facade published it while the
// implementation had no equivalent; dropping it during the move would have
// been a silent breaking change.
//
// It is meant for tool parameter schemas written as Go literals at init time,
// where a marshalling failure is a programming error rather than a runtime
// condition. Do not use it on request-time data.
func MustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
