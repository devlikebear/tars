package agentloop

import (
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/pkg/llm"
)

// New constructs a Loop.
//
// It is kept because the deleted alias facade published the constructor under
// this name while the implementation called it NewLoop. Dropping it during
// the move would have been a silent breaking change for anyone who wrote
// agentloop.New — including examples/min-agent and the documented "minimal
// agent shape".
//
// New and NewLoop are the same call; New is the name to prefer, since it is
// the one the documentation and examples use.
func New(client llm.Client, registry *tool.Registry, hooks ...Hook) *Loop {
	return NewLoop(client, registry, hooks...)
}
