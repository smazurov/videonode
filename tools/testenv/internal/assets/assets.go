// Package assets exposes the embedded SKILL.md tree and hook
// template that the install subcommand writes into a project.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed skills/*/SKILL.md
var skillsRaw embed.FS

// HooksTemplate is the settings.json hook template written by the install subcommand.
//
//go:embed hooks/settings.json.tmpl
var HooksTemplate []byte

// Skills is the embedded SKILL.md tree rooted so that
// fs.WalkDir(Skills, ".") yields entries like "testenv-up/SKILL.md".
var Skills = mustSub(skillsRaw, "skills")

func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
