// Package computeruse drives a native GUI toward a plain-language goal
// without an LLM in the loop: an accessibility-tree Driver observes the
// window, TypeSafe's Jev picks one operation and one target per step, and
// deterministic gates decide whether to act, look closer, stop, or ask the
// caller to confirm a risky action.
package computeruse
