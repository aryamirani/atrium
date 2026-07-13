package agent

import (
	"regexp"
	"strings"
)

// Live-spinner detection for claude (2.1.207).
//
// Claude's busy footer marker ("esc to interrupt") is no longer guaranteed: 2.1.207
// made the footer below the input box a responsive, prioritized hint area, and when a
// session accrues contextual chips (a "PR #NNN" link, "ctrl+t to hide tasks", "N shell",
// "1 monitor", "↓ to manage") they crowd the lower-priority "esc to interrupt" hint out.
// A foreground turn (no in-flight sub-agents) then shows no busy marker at all, and the
// poller — which never trusts the bare "working" hook latch (the #46 stuck-file guard) —
// settles it to idle mid-work.
//
// The surviving positive proof of work is the spinner STATUS LINE claude renders just
// above the input box for the whole turn: "<glyph> <Gerund>… (<elapsed> · …)". The glyph
// (✻ ✽ ✢ ✶ * ·) and gerund vary, so only the structure is matchable — and its
// discriminator, confirmed live against 2.1.207 sessions (2026-07-13), is the elapsed
// timer sitting immediately after "(" and followed by a " · " middot:
//
//	✽ Opening PR and running CI… (14m 24s · ↓ 34.6k tokens)
//
// A settled turn instead shows a PAST-TENSE summary with no parens/middot ("✻ Worked for
// 20m 57s"); a bare tool duration has no middot ("done (2m 38s)"); a sub-agent completion
// has a middot but the duration is last, not right after "(" ("Done (15 tool uses · … ·
// 2m 20s)"). The gerund ellipsis is U+2026, which normal program output/prose does not
// emit before a timer (it uses ASCII "..."), so requiring it is a second FP guard on top
// of the middot. Detection is confined to aboveBoxBlock — the live status band directly
// above the box, blank-delimited from the transcript — because a pane can carry the same
// signature quoted in scrollback; a whole-pane match would pin such a session working
// forever (the #46 failure). Like every heuristic here it fails safe: a rewording upstream
// makes the pane read idle (the pre-fix behavior), never a wrong action, and the
// VerifiedVersion drift guard flags the next minor bump for re-verification.

// claudeSpinnerRegex matches the live spinner status line's signature: the gerund ellipsis
// (U+2026), then an in-paren elapsed timer whose seconds sit immediately after "(" and are
// followed by a " · " (U+00B7) middot. Explicit code points keep the pattern independent of
// the source file's byte encoding.
var claudeSpinnerRegex = regexp.MustCompile(`\x{2026}\s*\((?:\d+h )?(?:\d+m )?\d+s \x{00b7} `)

// claudeSpinnerWorking backs the claude adapter's LiveSpinner: it reports whether the live
// status block above the input box shows an animating spinner. Matching is per-line (never
// flattened) so a transcript line ending in "…" cannot combine with a different line's
// "(Ns · )" into a false signature.
func claudeSpinnerWorking(content string) bool {
	block, ok := aboveBoxBlock(content)
	if !ok {
		return false
	}
	for _, line := range strings.Split(block, "\n") {
		if claudeSpinnerRegex.MatchString(line) {
			return true
		}
	}
	return false
}
