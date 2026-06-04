// Package builtin provides ready-to-use Tool implementations for agents.
// These are minimal, dependency-free tools intended for learning/demo —
// the calculator is a hand-rolled Pratt parser; the search is a mock.
package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Calculator evaluates arithmetic expressions with + - * / and parens.
// Supports integers, decimals, unary minus.
type Calculator struct{}

// NewCalculator returns a Calculator tool.
func NewCalculator() *Calculator { return &Calculator{} }

// Name implements agents.Tool.
func (Calculator) Name() string { return "calculator" }

// Description implements agents.Tool.
func (Calculator) Description() string {
	return "Evaluate an arithmetic expression with + - * / and parentheses. Returns the numeric result as a string."
}

// Schema implements agents.Tool.
func (Calculator) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"expr": {"type": "string", "description": "Arithmetic expression to evaluate"}
		},
		"required": ["expr"]
	}`)
}

// Execute parses and evaluates expr from args.
func (Calculator) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Expr string `json:"expr"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("calculator: bad args: %w", err)
	}
	if strings.TrimSpace(p.Expr) == "" {
		return "", errors.New("calculator: expr is required")
	}
	v, err := evalExpr(p.Expr)
	if err != nil {
		return "", err
	}
	return formatNumber(v), nil
}

// formatNumber renders without trailing zeros: 3.0 -> "3", 2.50 -> "2.5".
func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// --- Pratt parser ---------------------------------------------------------

type parser struct {
	src string
	pos int
}

func evalExpr(src string) (float64, error) {
	p := &parser{src: strings.ReplaceAll(src, " ", "")}
	v, err := p.parseExpr(0)
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("calculator: unexpected trailing %q", p.src[p.pos:])
	}
	return v, nil
}

// parseExpr parses with operator precedence (Pratt).
func (p *parser) parseExpr(minPrec int) (float64, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.src) {
		op := p.src[p.pos]
		prec := opPrec(op)
		if prec == 0 || prec < minPrec {
			break
		}
		p.pos++
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return 0, err
		}
		left, err = applyOp(op, left, right)
		if err != nil {
			return 0, err
		}
	}
	return left, nil
}

func (p *parser) parsePrefix() (float64, error) {
	if p.pos >= len(p.src) {
		return 0, errors.New("calculator: unexpected end of input")
	}
	c := p.src[p.pos]
	switch {
	case c == '(':
		p.pos++
		v, err := p.parseExpr(0)
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return 0, errors.New("calculator: missing )")
		}
		p.pos++
		return v, nil
	case c == '-':
		p.pos++
		v, err := p.parsePrefix()
		if err != nil {
			return 0, err
		}
		return -v, nil
	case (c >= '0' && c <= '9') || c == '.':
		start := p.pos
		for p.pos < len(p.src) && (isDigit(p.src[p.pos]) || p.src[p.pos] == '.') {
			p.pos++
		}
		return strconv.ParseFloat(p.src[start:p.pos], 64)
	default:
		return 0, fmt.Errorf("calculator: unexpected char %q", c)
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func opPrec(c byte) int {
	switch c {
	case '+', '-':
		return 1
	case '*', '/':
		return 2
	default:
		return 0
	}
}

func applyOp(op byte, l, r float64) (float64, error) {
	switch op {
	case '+':
		return l + r, nil
	case '-':
		return l - r, nil
	case '*':
		return l * r, nil
	case '/':
		if r == 0 {
			return 0, errors.New("calculator: division by zero")
		}
		return l / r, nil
	}
	return 0, fmt.Errorf("calculator: unknown op %q", op)
}
