package browserplugin

import "github.com/devlikebear/tars/internal/plugin"

func init() {
	plugin.RegisterBuiltin(&Plugin{})
}
