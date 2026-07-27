package workflowpolicy

import (
	"strings"
	"unicode"
)

type shellInvocation struct {
	executable       int
	workingDirectory string
}

type shellCommandParser struct {
	commands         [][]string
	command          []string
	word             strings.Builder
	workingDirectory string
	quote            byte
	comment          bool
}

func splitShellCommands(script string) [][]string {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	parser := shellCommandParser{
		commands: make([][]string, 0, 4),
		command:  make([]string, 0, 8),
	}
	for index := 0; index < len(script); index++ {
		if parser.comment {
			parser.consumeComment(script[index])
			continue
		}
		if parser.quote != 0 {
			index = parser.consumeQuoted(script, index)
			continue
		}
		index = parser.consumeUnquoted(script, index)
	}
	parser.flushCommand(false)
	return parser.commands
}

func (p *shellCommandParser) consumeComment(character byte) {
	if character == '\n' {
		p.comment = false
		p.flushCommand(true)
	}
}

func (p *shellCommandParser) consumeQuoted(script string, index int) int {
	character := script[index]
	if character == p.quote {
		p.quote = 0
		return index
	}
	if p.quote != '"' || character != '\\' || index+1 >= len(script) {
		p.word.WriteByte(character)
		return index
	}
	index++
	if script[index] != '\n' {
		p.word.WriteByte(script[index])
	}
	return index
}

func (p *shellCommandParser) consumeUnquoted(script string, index int) int {
	character := script[index]
	switch character {
	case '\'', '"':
		p.quote = character
	case '\\':
		return p.consumeUnquotedEscape(script, index)
	case '#':
		return p.consumeUnquotedComment(character, index)
	case ';':
		p.flushCommand(true)
	case '|', '&':
		return p.consumeUnquotedLogicalOperator(script, character, index)
	case '(', ')':
		p.flushCommand(false)
	default:
		return p.consumeUnquotedText(character, index)
	}
	return index
}

func (p *shellCommandParser) consumeUnquotedEscape(script string, index int) int {
	if index+1 >= len(script) {
		p.word.WriteByte(script[index])
		return index
	}
	if script[index+1] == '\n' {
		return index + 1
	}
	index++
	p.word.WriteByte(script[index])
	return index
}

func (p *shellCommandParser) consumeUnquotedComment(character byte, index int) int {
	if p.word.Len() == 0 {
		p.comment = true
		return index
	}
	p.word.WriteByte(character)
	return index
}

func (p *shellCommandParser) consumeUnquotedLogicalOperator(script string, character byte, index int) int {
	logicalAnd := character == '&' && index+1 < len(script) && script[index+1] == '&'
	p.flushCommand(logicalAnd)
	if index+1 < len(script) && script[index+1] == character {
		index++
	}
	return index
}

func (p *shellCommandParser) consumeUnquotedText(character byte, index int) int {
	if unicode.IsSpace(rune(character)) {
		p.flushWord()
		if character == '\n' {
			p.flushCommand(true)
		}
		return index
	}
	p.word.WriteByte(character)
	return index
}

func (p *shellCommandParser) flushWord() {
	if p.word.Len() == 0 {
		return
	}
	p.command = append(p.command, p.word.String())
	p.word.Reset()
}

func (p *shellCommandParser) flushCommand(carryDirectory bool) {
	p.flushWord()
	if len(p.command) == 0 {
		return
	}
	command := p.command
	if p.workingDirectory != "" {
		command = append([]string{"env", "--chdir=" + p.workingDirectory}, command...)
	}
	p.commands = append(p.commands, command)
	if carryDirectory {
		p.carryWorkingDirectory()
	}
	p.command = make([]string, 0, 8)
}
