// Package toolschema holds the wire-level description of a callable tool:
// the shape a tool registry advertises and a model client sends.
//
// It is a leaf on purpose. Both pkg/tools (which produces schemas from a
// registry) and pkg/llm (which puts them on the wire) need these two types,
// and if either package owned them the other would have to import it -- so
// an application that only wants a tool registry would link every provider
// client, or one that only wants a model client would link the tool
// implementations. linetta's dependency gate caught exactly the first case
// when the definitions moved from internal/llm into pkg/llm. pkg/llm keeps
// ToolSchema and ToolFunctionSchema as aliases of these, so existing code
// reads and compiles as before.
package toolschema

import "encoding/json"

// FunctionSchema describes one callable tool. Parameters is a JSON Schema
// object; the model sees Description verbatim, so it is the main lever on
// whether a tool gets called correctly.
type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Schema wraps a function schema with its dispatch type. Type is "function"
// for every provider currently supported.
type Schema struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}
