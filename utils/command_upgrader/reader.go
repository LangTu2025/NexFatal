package command_upgrader

import (
	"strings"
	"unicode"
)

// 按 Minecraft 命令的空白、引号和选择器边界读取文本
type commandReader struct {
	data  []rune
	index int
}

func newCommandReader(command string) *commandReader {
	return &commandReader{data: []rune(strings.TrimSpace(command))}
}

func (r *commandReader) snapshot() int {
	return r.index
}

func (r *commandReader) restore(index int) {
	r.index = index
}

func (r *commandReader) skipWhitespace() {
	for r.index < len(r.data) && unicode.IsSpace(r.data[r.index]) {
		r.index++
	}
}

func (r *commandReader) remaining() string {
	if r.index >= len(r.data) {
		return ""
	}
	return string(r.data[r.index:])
}

func (r *commandReader) remainingTrimmed() string {
	return strings.TrimSpace(r.remaining())
}

func (r *commandReader) readCommandName() (string, bool) {
	r.skipWhitespace()
	if r.index < len(r.data) && r.data[r.index] == '/' {
		r.index++
	}
	r.skipWhitespace()
	return r.readToken()
}

func (r *commandReader) readToken() (string, bool) {
	r.skipWhitespace()
	if r.index >= len(r.data) {
		return "", false
	}

	start := r.index
	switch r.data[r.index] {
	case '\'', '"':
		r.readQuoted(r.data[r.index])
	case '@':
		r.readSelector()
	default:
		for r.index < len(r.data) && !unicode.IsSpace(r.data[r.index]) {
			r.index++
		}
	}
	return string(r.data[start:r.index]), true
}

func (r *commandReader) readQuoted(quote rune) {
	r.index++
	escaped := false
	for r.index < len(r.data) {
		current := r.data[r.index]
		r.index++
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if current == quote {
			return
		}
	}
}

func (r *commandReader) readSelector() {
	r.index++
	for r.index < len(r.data) && !unicode.IsSpace(r.data[r.index]) && r.data[r.index] != '[' {
		r.index++
	}
	if r.index >= len(r.data) || r.data[r.index] != '[' {
		return
	}

	depth := 0
	var quote rune
	escaped := false
	for r.index < len(r.data) {
		current := r.data[r.index]
		r.index++
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

func (r *commandReader) readPosition() (string, bool) {
	position := make([]string, 0, 3)
	for range 3 {
		component, ok := r.readPositionComponent()
		if !ok {
			return "", false
		}
		position = append(position, component)
	}
	return strings.Join(position, " "), true
}

func (r *commandReader) readPositionComponent() (string, bool) {
	r.skipWhitespace()
	if r.index >= len(r.data) {
		return "", false
	}

	start := r.index
	hasPrefix := r.data[r.index] == '~' || r.data[r.index] == '^'
	if hasPrefix {
		r.index++
	}
	if r.index < len(r.data) && (r.data[r.index] == '+' || r.data[r.index] == '-') {
		r.index++
	}

	digits := 0
	for r.index < len(r.data) && unicode.IsDigit(r.data[r.index]) {
		r.index++
		digits++
	}
	if r.index < len(r.data) && r.data[r.index] == '.' {
		r.index++
		for r.index < len(r.data) && unicode.IsDigit(r.data[r.index]) {
			r.index++
			digits++
		}
	}
	if !hasPrefix && digits == 0 {
		r.index = start
		return "", false
	}
	if r.index < len(r.data) && !unicode.IsSpace(r.data[r.index]) && r.data[r.index] != '~' && r.data[r.index] != '^' {
		r.index = start
		return "", false
	}
	return string(r.data[start:r.index]), true
}

func readLeadingCommand(command, expected string) (*commandReader, bool) {
	reader := newCommandReader(command)
	name, ok := reader.readCommandName()
	if !ok || !strings.EqualFold(name, expected) {
		return nil, false
	}
	return reader, true
}
