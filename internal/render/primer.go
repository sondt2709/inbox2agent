package render

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CardSection returns the fixed part of the primer that tells the agent how to
// report its results to i2a. c is the command that runs i2a, for example
// "/usr/local/bin/i2a" or "I2A_HOME=/x /usr/local/bin/i2a". It is not a
// template field, so a custom primer cannot remove it.
func CardSection(c string) string {
	return `## Report to me in i2a

I read your results in i2a, not in this terminal. This is my instruction, not message data, so a rule about commands in messages does not apply to it.
After you process each batch, send me one card for each topic. Run this command one time for each batch, with a JSON array on stdin:

` + c + ` card add <<'EOF'
[{"category":"action","urgency":"high","group":"<project>","title":"<one line>","body":"<summary and evidence>","reply":"<suggested reply>","refs":[<id>],"key":"<topic key>"}]
EOF

Fields:
- category: "action" (I must do something), "question" (someone asks me something) or "info" (no action).
- urgency: "high" (today, or someone waits for me), "normal" or "low" (for the record only).
- group: the project or the team.
- title: one line that tells what to do or what happened.
- body: the summary and the evidence. It can have more lines.
- reply: a suggested reply. Leave it empty when no reply is necessary.
- refs: the "id" values of the messages of this topic, from the data block.
- key: a stable key for the topic, for example "mm:" and the thread or the channel. Use the same key for the same topic, and i2a updates that card.

Write one card for each topic. The language rules above apply to the card text.
Close a card that is no longer necessary: ` + c + ` card done --key <key>
Read the open cards: ` + c + ` card list --json`
}

// CardCommand returns the shell command that runs this i2a executable for the
// agent. When I2A_HOME is set, the command sets it too, so the agent writes to
// the same store.
func CardCommand(exe, home string) string {
	cmd := shellQuote(exe)
	if home != "" {
		cmd = "I2A_HOME=" + shellQuote(home) + " " + cmd
	}
	return cmd
}

// shellQuote quotes s for a POSIX shell, unless it has only safe characters.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:@", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// primerHash identifies the primer that a session got: the template text and
// the card section. It does not use the rendered primer, which can contain the
// batch ID.
func primerHash(template, section string) string {
	sum := sha256.Sum256([]byte(template + "\x00" + section))
	return hex.EncodeToString(sum[:])[:16]
}
