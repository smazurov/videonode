package cmd

import (
	"fmt"

	"github.com/smazurov/videonode/tools/testenv/internal/envctl"
)

type UpCmd struct {
	Target string `enum:"host,sbc" default:"host" help:"Where to spawn the env."`
	Source string `enum:"fake,real" default:"fake" help:"Source mode."`
	Device string `default:"/dev/video0" help:"Device path when --source real."`
}

func (c *UpCmd) Run(ctx *Context) error {
	r, err := envctl.Up(ctx.Ctx, envctl.UpParams{
		StatePath: ctx.StatePath,
		Session:   ctx.SessionID,
		Target:    c.Target,
		Source:    c.Source,
		Device:    c.Device,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout(), "env %s up · slot %d · %s\n", r.EnvID, r.Slot, r.HTTPURL)
	fmt.Fprintf(stdout(), "  rtsp: %s\n  srt:  %s\n  data: %s\n  pid:  %d\n",
		r.RTSPURL, r.SRTURL, r.DataDir, r.PID)
	return nil
}
