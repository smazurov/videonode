// Package assets exposes the embedded SKILL.md tree and hook
// template that the install subcommand writes into a project.
package assets

import (
	"embed"
	_ "embed"
	"io/fs"
)

//go:embed skills/*/SKILL.md
var skillsRaw embed.FS

//go:embed hooks/settings.json.tmpl
var HooksTemplate []byte

// Skills is the embedded SKILL.md tree rooted so that
// fs.WalkDir(Skills, ".") yields entries like "testenv-up/SKILL.md".
var Skills fs.FS

func init() {
	sub, err := fs.Sub(skillsRaw, "skills")
	if err != nil {
		panic(err)
	}
	Skills = sub
}
