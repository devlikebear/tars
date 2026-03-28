package consoleassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist placeholder/index.html
var distDir embed.FS

func DistFS() fs.FS {
	sub, err := fs.Sub(distDir, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
