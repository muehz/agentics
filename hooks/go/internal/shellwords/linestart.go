package shellwords

// LineStarts reports, per physical line of command, whether the line
// provably begins as executable shell text: outside every quote, outside a
// heredoc body, and not continuing the previous line. It is the one quote
// tracker the heredoc pre-pass and command-guardian's heredoc strip share
// (#651). Anything it cannot prove is false, and a false line never opens
// a heredoc.
func LineStarts(command string) []bool {
	return readHeredocExtents(command).lineStarts()
}
