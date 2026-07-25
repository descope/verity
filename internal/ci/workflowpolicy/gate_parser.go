package workflowpolicy

import (
	"fmt"
	"slices"
	"strings"
)

type gateExpressionKind uint8

const (
	gateExpressionAtom gateExpressionKind = iota
	gateExpressionCall
	gateExpressionNot
	gateExpressionEqual
	gateExpressionNotEqual
	gateExpressionAnd
	gateExpressionOr
)

type gateExpression struct {
	kind     gateExpressionKind
	value    string
	children []gateExpression
}

type gateParser struct {
	tokens []gateToken
	index  int
}

func parseGateExpression(value string) (gateExpression, error) {
	tokens, err := lexGateExpression(value)
	if err != nil {
		return gateExpression{}, err
	}
	parser := gateParser{tokens: tokens}
	expression, err := parser.parseOr()
	if err != nil {
		return gateExpression{}, err
	}
	if parser.current().kind != gateTokenEOF {
		return gateExpression{}, fmt.Errorf("%w: trailing token %q", errGateExpression, parser.current().text)
	}
	return expression, nil
}

func (parser *gateParser) parseOr() (gateExpression, error) {
	return parser.parseBinary(parser.parseAnd, gateTokenOr, gateExpressionOr)
}

func (parser *gateParser) parseAnd() (gateExpression, error) {
	return parser.parseBinary(parser.parseUnary, gateTokenAnd, gateExpressionAnd)
}

func (parser *gateParser) parseBinary(
	parseOperand func() (gateExpression, error),
	tokenKind gateTokenKind,
	expressionKind gateExpressionKind,
) (gateExpression, error) {
	left, err := parseOperand()
	if err != nil {
		return gateExpression{}, err
	}
	children := []gateExpression{left}
	for parser.current().kind == tokenKind {
		parser.index++
		right, parseErr := parseOperand()
		if parseErr != nil {
			return gateExpression{}, parseErr
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return gateExpression{kind: expressionKind, children: children}, nil
}

func (parser *gateParser) parseUnary() (gateExpression, error) {
	if parser.current().kind == gateTokenNot {
		parser.index++
		child, err := parser.parseUnary()
		if err != nil {
			return gateExpression{}, err
		}
		return gateExpression{kind: gateExpressionNot, children: []gateExpression{child}}, nil
	}
	return parser.parseComparison()
}

func (parser *gateParser) parseComparison() (gateExpression, error) {
	left, err := parser.parsePrimary()
	if err != nil {
		return gateExpression{}, err
	}
	kind := parser.current().kind
	if kind != gateTokenEqual && kind != gateTokenNotEqual {
		return left, nil
	}
	parser.index++
	right, err := parser.parsePrimary()
	if err != nil {
		return gateExpression{}, err
	}
	expressionKind := gateExpressionEqual
	if kind == gateTokenNotEqual {
		expressionKind = gateExpressionNotEqual
	}
	return gateExpression{kind: expressionKind, children: []gateExpression{left, right}}, nil
}

func (parser *gateParser) parsePrimary() (gateExpression, error) {
	token := parser.current()
	switch token.kind {
	case gateTokenLeftParen:
		parser.index++
		expression, err := parser.parseOr()
		if err != nil {
			return gateExpression{}, err
		}
		if parser.current().kind != gateTokenRightParen {
			return gateExpression{}, fmt.Errorf("%w: missing closing parenthesis", errGateExpression)
		}
		parser.index++
		return expression, nil
	case gateTokenAtom:
		parser.index++
		if parser.current().kind != gateTokenLeftParen {
			return gateExpression{kind: gateExpressionAtom, value: token.text}, nil
		}
		parser.index++
		if parser.current().kind != gateTokenRightParen {
			return gateExpression{}, fmt.Errorf("%w: function arguments are unsupported", errGateExpression)
		}
		parser.index++
		return gateExpression{kind: gateExpressionCall, value: token.text}, nil
	default:
		return gateExpression{}, fmt.Errorf("%w: unexpected token %q", errGateExpression, token.text)
	}
}

func (parser *gateParser) current() gateToken {
	return parser.tokens[parser.index]
}

func gatesEquivalent(actual, expected string) bool {
	actualExpression, err := parseGateExpression(actual)
	if err != nil {
		return false
	}
	expectedExpression, err := parseGateExpression(expected)
	return err == nil && actualExpression.canonical() == expectedExpression.canonical()
}

func (expression gateExpression) canonical() string {
	children := make([]string, 0, len(expression.children))
	for _, child := range expression.children {
		children = append(children, child.canonical())
	}
	switch expression.kind {
	case gateExpressionAtom:
		return "atom(" + expression.value + ")"
	case gateExpressionCall:
		return "call(" + expression.value + ")"
	case gateExpressionNot:
		return "not(" + children[0] + ")"
	case gateExpressionEqual, gateExpressionNotEqual:
		slices.Sort(children)
		return fmt.Sprintf("compare(%d,%s)", expression.kind, strings.Join(children, ","))
	case gateExpressionAnd, gateExpressionOr:
		children = flattenGateChildren(expression.kind, expression.children)
		slices.Sort(children)
		return fmt.Sprintf("logic(%d,%s)", expression.kind, strings.Join(children, ","))
	default:
		return "invalid"
	}
}

func flattenGateChildren(kind gateExpressionKind, expressions []gateExpression) []string {
	children := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		if expression.kind == kind {
			children = append(children, flattenGateChildren(kind, expression.children)...)
			continue
		}
		children = append(children, expression.canonical())
	}
	return children
}
