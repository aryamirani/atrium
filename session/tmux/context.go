package tmux

import "fmt"

// context.go — feeding the in-session context bar. Atrium pushes each session's
// live, pre-rendered header string into tmux user options (@atrium_name /
// @atrium_left); the managed config's status-line format and set-titles-string read
// them. The string is composed by the ui layer (tmux #[...] markup), so this file
// stays presentation-agnostic.

// SetContext pushes the context-bar strings into this session's tmux user options
// (@atrium_name / @atrium_left) in a single batched command, and — only when a client
// is attached — a refresh-client -S so the live status line repaints. It is a no-op
// when the values are unchanged since the last push, so the per-second metadata tick
// costs a string comparison rather than a subprocess when nothing moved. name also
// drives the terminal title via set-titles-string.
//
// The refresh is gated on attachment because a detached session is clientless (geometry
// is driven server-side, no tmux client). refresh-client then errors with "no client",
// which used to abort the whole batch — and since ctxSet is recorded only on success,
// the values never cached, so every detached session re-ran this subprocess on EVERY
// metadata tick, stalling the main event loop (input lag). Setting the options needs no
// client and always succeeds, so we cache off that; a detached session's bar has no
// client to repaint and picks up the options the moment the user attaches.
func (t *Session) SetContext(name, left string) error {
	if t.ctxSet && t.ctxName == name && t.ctxLeft == left {
		return nil
	}
	target := t.snapshotName()
	ctx, cancel := t.opContext()
	defer cancel()
	args := []string{
		"set-option", "-t", target, "@atrium_name", name, ";",
		"set-option", "-t", target, "@atrium_left", left,
	}
	if t.attached.Load() {
		args = append(args, ";", "refresh-client", "-S")
	}
	if err := t.cmdExec.Run(tmuxCommand(ctx, args...)); err != nil {
		return fmt.Errorf("failed to set tmux session context: %w", err)
	}
	t.ctxName, t.ctxLeft, t.ctxSet = name, left, true
	return nil
}
