// Package agentloop exposes TARS' reusable LLM tool-calling loop. It turns a
// provider-normalized llm.Client plus a tools.Registry into an iterative agent
// loop that calls tools, appends tool results, detects repeated tool-call
// patterns, and emits audit hooks.
package agentloop
