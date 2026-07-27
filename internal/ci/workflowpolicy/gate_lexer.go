package workflowpolicy

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var errGateExpression = errors.New("invalid workflow gate expression")

type gateTokenKind uint8

const (
	gateTokenEOF gateTokenKind = iota
	gateTokenAtom
	gateTokenAnd
	gateTokenOr
	gateTokenNot
	gateTokenEqual
	gateTokenNotEqual
	gateTokenLeftParen
	gateTokenRightParen
)

type gateToken struct {
	kind gateTokenKind
	text string
}

func lexGateExpression(value string) ([]gateToken, error) {
	body, err := gateExpressionBody(value)
	if err != nil {
		return nil, err
	}
	tokens := make([]gateToken, 0, 16)
	for index := 0; index < len(body); {
		if unicode.IsSpace(rune(body[index])) {
			index++
			continue
		}
		if token, width, ok := gateOperator(body[index:]); ok {
			tokens = append(tokens, token)
			index += width
			continue
		}
		if body[index] == '\'' || body[index] == '"' {
			token, next, scanErr := scanGateString(body, index)
			if scanErr != nil {
				return nil, scanErr
			}
			tokens = append(tokens, token)
			index = next
			continue
		}
		start := index
		for index < len(body) && !gateDelimiter(body[index]) {
			index++
		}
		if start == index {
			return nil, fmt.Errorf("%w: unexpected character %q", errGateExpression, body[index])
		}
		tokens = append(tokens, gateToken{kind: gateTokenAtom, text: strings.ToLower(body[start:index])})
	}
	tokens = append(tokens, gateToken{kind: gateTokenEOF})
	return tokens, nil
}

func gateExpressionBody(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "${{") {
		if !strings.HasSuffix(value, "}}") {
			return "", fmt.Errorf("%w: unterminated expression", errGateExpression)
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${{"), "}}"))
	}
	if value == "" {
		return "", fmt.Errorf("%w: empty expression", errGateExpression)
	}
	return value, nil
}

func gateOperator(value string) (gateToken, int, bool) {
	for operator, token := range map[string]gateTokenKind{
		"&&": gateTokenAnd,
		"||": gateTokenOr,
		"==": gateTokenEqual,
		"!=": gateTokenNotEqual,
	} {
		if strings.HasPrefix(value, operator) {
			return gateToken{kind: token, text: operator}, len(operator), true
		}
	}
	switch value[0] {
	case '!':
		return gateToken{kind: gateTokenNot, text: "!"}, 1, true
	case '(':
		return gateToken{kind: gateTokenLeftParen, text: "("}, 1, true
	case ')':
		return gateToken{kind: gateTokenRightParen, text: ")"}, 1, true
	default:
		return gateToken{}, 0, false
	}
}

func scanGateString(value string, start int) (gateToken, int, error) {
	quote := value[start]
	for index := start + 1; index < len(value); index++ {
		if value[index] == quote {
			return gateToken{kind: gateTokenAtom, text: "string:" + value[start+1:index]}, index + 1, nil
		}
	}
	return gateToken{}, 0, fmt.Errorf("%w: unterminated string", errGateExpression)
}

func gateDelimiter(value byte) bool {
	return unicode.IsSpace(rune(value)) || strings.ContainsRune("&|=!()'\"", rune(value))
}
