package nt

import "strings"

// IsTraceHeader reports whether line is one of the carrier header fragments
// emitted by TraceRoute. The header is intentionally kept without a newline so
// that it can share a line with the following traceroute command.
//
// Stop-reason lines can also contain the word "ICMP". Matching the complete
// header suffix keeps those lines from being mistaken for fragments.
func IsTraceHeader(line string) bool {
	plain := strings.ToLower(strings.Join(strings.Fields(stripAnsi(line)), " "))
	return strings.HasSuffix(plain, " - icmp v4 -") || strings.HasSuffix(plain, " - icmp v6 -")
}

// FormatTraceOutput converts TraceResult output lines into the historical
// human-readable stream. A carrier header is joined only with an immediately
// following traceroute command; every other logical line, including a trace
// stop reason, is terminated with a newline.
//
// The function always terminates non-empty output with a newline. This makes
// header-only and error results safe to append to another route section.
func FormatTraceOutput(lines []string) string {
	var output strings.Builder
	for index, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// NTrace-core emits informational terminal lines after a completed
		// route. They are redundant in the ECS report; keep other stop reasons.
		if isInformationalTerminalStopReason(line) {
			continue
		}
		output.WriteString(line)
		next, ok := nextTraceLine(lines, index)
		if !(IsTraceHeader(line) && ok && isTraceBody(next)) {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func isInformationalTerminalStopReason(line string) bool {
	plain := strings.ToLower(strings.TrimSpace(stripAnsi(line)))
	return strings.HasPrefix(plain, "trace stopped: destination reached at hop ") ||
		strings.HasPrefix(plain, "trace stopped: maximum hops reached at hop ")
}

func nextTraceLine(lines []string, index int) (string, bool) {
	for index++; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			return line, true
		}
	}
	return "", false
}

func isTraceBody(line string) bool {
	plain := strings.ToLower(strings.TrimSpace(stripAnsi(line)))
	return strings.HasPrefix(plain, "traceroute to ")
}
