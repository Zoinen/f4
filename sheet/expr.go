package sheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// TrueValue is the numeric result of a successful comparison. Logical values
// are all-ones so that the bitwise operators double as logical ones, exactly
// as in the calculator this dialect comes from.
const TrueValue = float64(0xFFFFFFFF)

// Error is a formula error carrying the offset inside the expression where it
// was detected, which lets the UI put the cursor on the offending operator.
type Error struct {
	Message string
	Pos     int
}

func (e *Error) Error() string { return e.Message }

func errorAt(pos int, format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Pos: pos}
}

// Env resolves references while an expression is evaluated.
type Env interface {
	// CellValue returns the numeric value of a single cell. Empty cells are
	// zero and text cells are an error.
	CellValue(ref Ref) (float64, error)
	// RangeValues returns the values inside a rectangle, skipping empty and
	// text cells rather than treating them as zero.
	RangeValues(from, to Ref) ([]float64, error)
}

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenNumber
	tokenIdent
	tokenOp
)

type token struct {
	kind   tokenKind
	text   string
	number float64
	pos    int
}

var siMultipliers = map[string]float64{
	"y": 1e-24, "z": 1e-21, "a": 1e-18, "f": 1e-15, "p": 1e-12,
	"n": 1e-9, "u": 1e-6, "m": 1e-3, "c": 1e-2, "d": 1e-1,
	"da": 1e1, "k": 1e3, "M": 1e6, "G": 1e9, "T": 1e12,
	"P": 1e15, "E": 1e18, "Z": 1e21, "Y": 1e24,
}

// functionArity lists every supported function. -1 marks the variadic
// aggregate functions, which require parentheses and accept ranges.
var functionArity = map[string]int{
	"pi":  0,
	"sin": 1, "cos": 1, "tan": 1, "tg": 1, "cotan": 1, "ctg": 1,
	"sec": 1, "cosec": 1, "arcsin": 1, "asin": 1, "arccos": 1, "acos": 1,
	"atan": 1, "arctan": 1, "actg": 1, "arccotan": 1, "arcsec": 1, "arccosec": 1,
	"rad": 1, "radg": 1, "deg": 1, "grad": 1,
	"sh": 1, "sinh": 1, "ch": 1, "cosh": 1, "th": 1, "tanh": 1,
	"cth": 1, "cotanh": 1, "arch": 1, "arccosh": 1, "ash": 1, "arcsinh": 1,
	"ath": 1, "arctanh": 1,
	"exp": 1, "fact": 1, "lg": 1, "ln": 1, "sqr": 1, "sqrt": 1,
	"round": 1, "sign": 1, "abs": 1,
	"log": 2, "root": 2,
	"if":  3,
	"sum": -1, "mul": -1,
}

func isWordOperator(word string) bool {
	switch strings.ToLower(word) {
	case "div", "mod", "shr", "shl", "and", "or":
		return true
	}
	return false
}

func tokenize(src string) ([]token, error) {
	var tokens []token
	runes := []rune(src)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == ' ' || r == '\t':
			i++
		case r >= '0' && r <= '9' || r == '.' || r == '$':
			tok, next, err := scanNumber(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
		case isRefStart(r):
			start := i
			for i < len(runes) && isRefBody(runes[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokenIdent, text: string(runes[start:i]), pos: start})
		default:
			op, width := scanOperator(runes, i)
			if width == 0 {
				return nil, errorAt(i, "unexpected character %q", string(r))
			}
			tokens = append(tokens, token{kind: tokenOp, text: op, pos: i})
			i += width
		}
	}
	tokens = append(tokens, token{kind: tokenEOF, pos: len(runes)})
	return tokens, nil
}

func scanOperator(runes []rune, i int) (string, int) {
	if i+1 < len(runes) {
		pair := string(runes[i : i+2])
		switch pair {
		case ">=", "<=", "!=", "<>", ">>", "<<":
			return pair, 2
		}
	}
	switch runes[i] {
	case '=', '#', '<', '>', '+', '-', '|', '&', '*', '/', '%', '\\', '^', ':', '(', ')', ',', '~':
		return string(runes[i]), 1
	}
	return "", 0
}

// scanNumber accepts decimals with an exponent and an optional SI multiplier,
// hexadecimal ($10.1, 0x10.1, 10.1h), octal (777o, 777q) and binary (1011b).
func scanNumber(runes []rune, start int) (token, int, error) {
	i := start
	if runes[i] == '$' {
		i++
		digitsStart := i
		for i < len(runes) && (isHexDigit(runes[i]) || runes[i] == '.') {
			i++
		}
		value, err := parseRadix(string(runes[digitsStart:i]), 16)
		if err != nil {
			return token{}, 0, errorAt(start, "bad hexadecimal number")
		}
		return token{kind: tokenNumber, number: value, pos: start}, i, nil
	}
	if runes[i] == '0' && i+1 < len(runes) && (runes[i+1] == 'x' || runes[i+1] == 'X') {
		i += 2
		digitsStart := i
		for i < len(runes) && (isHexDigit(runes[i]) || runes[i] == '.') {
			i++
		}
		value, err := parseRadix(string(runes[digitsStart:i]), 16)
		if err != nil {
			return token{}, 0, errorAt(start, "bad hexadecimal number")
		}
		return token{kind: tokenNumber, number: value, pos: start}, i, nil
	}

	for i < len(runes) && (isAlphaNumeric(runes[i]) || runes[i] == '.') {
		if (runes[i] == 'e' || runes[i] == 'E') && i+1 < len(runes) &&
			(runes[i+1] == '+' || runes[i+1] == '-') && i+2 < len(runes) && isDigit(runes[i+2]) {
			i += 2
			continue
		}
		i++
	}
	text := string(runes[start:i])

	if value, ok := parseRadixSuffix(text); ok {
		return token{kind: tokenNumber, number: value, pos: start}, i, nil
	}

	// Longest decimal prefix wins; the remainder must be an SI multiplier.
	for end := len(text); end > 0; end-- {
		value, err := strconv.ParseFloat(text[:end], 64)
		if err != nil {
			continue
		}
		suffix := text[end:]
		if suffix == "" {
			return token{kind: tokenNumber, number: value, pos: start}, i, nil
		}
		if factor, ok := siMultipliers[suffix]; ok {
			return token{kind: tokenNumber, number: value * factor, pos: start}, i, nil
		}
	}
	return token{}, 0, errorAt(start, "bad number %q", text)
}

func parseRadixSuffix(text string) (float64, bool) {
	if len(text) < 2 {
		return 0, false
	}
	body, suffix := text[:len(text)-1], text[len(text)-1]
	base := 0
	switch suffix {
	case 'h', 'H':
		base = 16
	case 'o', 'O', 'q', 'Q':
		base = 8
	case 'b', 'B':
		base = 2
	default:
		return 0, false
	}
	value, err := parseRadix(body, base)
	if err != nil {
		return 0, false
	}
	return value, true
}

// parseRadix parses an unsigned number in the given base, allowing a
// fractional part after the decimal point.
func parseRadix(text string, base int) (float64, error) {
	if text == "" {
		return 0, fmt.Errorf("empty number")
	}
	integer, fraction, _ := strings.Cut(text, ".")
	if integer == "" && fraction == "" {
		return 0, fmt.Errorf("empty number")
	}
	value := 0.0
	if integer != "" {
		parsed, err := strconv.ParseUint(integer, base, 64)
		if err != nil {
			return 0, err
		}
		value = float64(parsed)
	}
	scale := 1.0
	for _, r := range fraction {
		digit := digitValue(r)
		if digit < 0 || digit >= base {
			return 0, fmt.Errorf("bad digit %q", string(r))
		}
		scale /= float64(base)
		value += float64(digit) * scale
	}
	return value, nil
}

func digitValue(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return -1
}

func isDigit(r rune) bool    { return r >= '0' && r <= '9' }
func isHexDigit(r rune) bool { return digitValue(r) >= 0 }
func isAlphaNumeric(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// Expression nodes.
type node interface{}

type numberNode struct{ value float64 }

type refNode struct {
	ref Ref
	pos int
}

type rangeNode struct {
	from, to Ref
	pos      int
}

type unaryNode struct {
	op  string
	arg node
	pos int
}

type binaryNode struct {
	op          string
	left, right node
	pos         int
}

type callNode struct {
	name string
	args []node
	pos  int
}

// precedence of the infix operators, lowest binding first.
func precedence(op string) int {
	switch strings.ToLower(op) {
	case "=", "#", "!=", "<>", "<", ">", ">=", "<=":
		return 1
	case "+", "-", "|", "&", "and", "or":
		return 2
	case "*", "/", "div", "%", "mod", "\\", ">>", "shr", "<<", "shl":
		return 3
	case "^":
		return 4
	case ":":
		return 5
	}
	return 0
}

type parser struct {
	tokens []token
	pos    int
}

// Parse turns an expression into an evaluable tree. The '=' that marks a
// formula cell must already be stripped.
func Parse(src string) (node, error) {
	tokens, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	tree, err := p.parseExpression(1)
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokenEOF {
		return nil, errorAt(tok.pos, "unexpected %q", tok.text)
	}
	return tree, nil
}

func (p *parser) peek() token { return p.tokens[p.pos] }

func (p *parser) next() token {
	tok := p.tokens[p.pos]
	if tok.kind != tokenEOF {
		p.pos++
	}
	return tok
}

func (p *parser) parseExpression(minPrecedence int) (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		opText := tok.text
		if tok.kind == tokenIdent && isWordOperator(opText) {
			opText = strings.ToLower(opText)
		} else if tok.kind != tokenOp {
			return left, nil
		}
		level := precedence(opText)
		if level < minPrecedence {
			return left, nil
		}
		p.next()
		right, err := p.parseExpression(level + 1)
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: strings.ToLower(opText), left: left, right: right, pos: tok.pos}
	}
}

func (p *parser) parseUnary() (node, error) {
	tok := p.peek()
	if tok.kind == tokenOp {
		switch tok.text {
		case "+", "-", "~":
			p.next()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &unaryNode{op: tok.text, arg: arg, pos: tok.pos}, nil
		}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	tok := p.next()
	switch tok.kind {
	case tokenNumber:
		return &numberNode{value: tok.number}, nil
	case tokenOp:
		if tok.text == "(" {
			inner, err := p.parseExpression(1)
			if err != nil {
				return nil, err
			}
			if closing := p.next(); closing.text != ")" {
				return nil, errorAt(closing.pos, "expected ')'")
			}
			return inner, nil
		}
		return nil, errorAt(tok.pos, "unexpected %q", tok.text)
	case tokenIdent:
		name := strings.ToLower(tok.text)
		if arity, ok := functionArity[name]; ok {
			return p.parseCall(name, arity, tok.pos)
		}
		if ref, ok := ParseRef(tok.text); ok {
			return &refNode{ref: ref, pos: tok.pos}, nil
		}
		return nil, errorAt(tok.pos, "unknown name %q", tok.text)
	}
	return nil, errorAt(tok.pos, "unexpected end of expression")
}

func (p *parser) parseCall(name string, arity int, pos int) (node, error) {
	call := &callNode{name: name, pos: pos}
	if p.peek().kind == tokenOp && p.peek().text == "(" {
		p.next()
		if p.peek().kind == tokenOp && p.peek().text == ")" {
			p.next()
			return p.finishCall(call, arity, pos)
		}
		for {
			arg, err := p.parseArgument(name)
			if err != nil {
				return nil, err
			}
			call.args = append(call.args, arg)
			separator := p.next()
			if separator.text == ")" {
				break
			}
			if separator.text != "," {
				return nil, errorAt(separator.pos, "expected ',' or ')'")
			}
		}
		return p.finishCall(call, arity, pos)
	}
	if arity < 0 {
		return nil, errorAt(pos, "%s requires parentheses", name)
	}
	// Prefix form: the function grabs exactly its operands, binding tighter
	// than any infix operator, which is why "log 2+1" means log(2, +1) - the
	// plus loses its left operand and turns into a unary sign.
	for i := 0; i < arity; i++ {
		arg, err := p.parsePrefixArgument()
		if err != nil {
			return nil, err
		}
		call.args = append(call.args, arg)
	}
	return p.finishCall(call, arity, pos)
}

// parsePrefixArgument reads one operand of a function written without
// parentheses. Operands bind at the unary level, with one exception: the
// comparison operators sit at the very bottom of the precedence table and are
// therefore still allowed to glue an operand together, so that
// "if 3<5 log 2 8 4" means if(3<5, log(2,8), 4).
func (p *parser) parsePrefixArgument() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		if tok.kind != tokenOp || precedence(tok.text) != 1 {
			return left, nil
		}
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: strings.ToLower(tok.text), left: left, right: right, pos: tok.pos}
	}
}

func (p *parser) finishCall(call *callNode, arity int, pos int) (node, error) {
	if arity >= 0 && len(call.args) != arity {
		return nil, errorAt(pos, "%s expects %d argument(s)", call.name, arity)
	}
	if arity < 0 && len(call.args) == 0 {
		return nil, errorAt(pos, "%s expects at least one argument", call.name)
	}
	return call, nil
}

// parseArgument recognises rectangular ranges, which are only meaningful as
// direct arguments of the aggregate functions.
func (p *parser) parseArgument(function string) (node, error) {
	if function == "sum" || function == "mul" {
		if rangeExpr, ok := p.tryRange(); ok {
			return rangeExpr, nil
		}
	}
	return p.parseExpression(1)
}

func (p *parser) tryRange() (node, bool) {
	first := p.tokens[p.pos]
	if first.kind != tokenIdent {
		return nil, false
	}
	from, ok := ParseRef(first.text)
	if !ok {
		return nil, false
	}
	colon := p.tokens[p.pos+1]
	if colon.kind != tokenOp || colon.text != ":" {
		return nil, false
	}
	second := p.tokens[p.pos+2]
	if second.kind != tokenIdent {
		return nil, false
	}
	to, ok := ParseRef(second.text)
	if !ok {
		return nil, false
	}
	p.pos += 3
	return &rangeNode{from: from, to: to, pos: first.pos}, true
}

// Eval computes the value of a parsed expression.
func Eval(tree node, env Env) (float64, error) {
	switch n := tree.(type) {
	case *numberNode:
		return n.value, nil
	case *refNode:
		if env == nil {
			return 0, errorAt(n.pos, "references are not available here")
		}
		value, err := env.CellValue(n.ref)
		if err != nil {
			return 0, errorAt(n.pos, "%s: %v", n.ref.String(), err)
		}
		return value, nil
	case *rangeNode:
		return 0, errorAt(n.pos, "a range is only allowed inside sum() or mul()")
	case *unaryNode:
		return evalUnary(n, env)
	case *binaryNode:
		return evalBinary(n, env)
	case *callNode:
		return evalCall(n, env)
	}
	return 0, &Error{Message: "empty expression"}
}

func evalUnary(n *unaryNode, env Env) (float64, error) {
	value, err := Eval(n.arg, env)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case "+":
		return value, nil
	case "-":
		return -value, nil
	case "~":
		return float64(^toUint32(value)), nil
	}
	return 0, errorAt(n.pos, "unknown operator %q", n.op)
}

func evalBinary(n *binaryNode, env Env) (float64, error) {
	left, err := Eval(n.left, env)
	if err != nil {
		return 0, err
	}
	right, err := Eval(n.right, env)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case "+":
		return left + right, nil
	case "-":
		return left - right, nil
	case "*":
		return left * right, nil
	case "/":
		if right == 0 {
			return 0, errorAt(n.pos, "division by zero")
		}
		return left / right, nil
	case "div", "\\":
		if right == 0 {
			return 0, errorAt(n.pos, "division by zero")
		}
		return math.Trunc(left / right), nil
	case "%", "mod":
		if right == 0 {
			return 0, errorAt(n.pos, "division by zero")
		}
		return left - math.Trunc(left/right)*right, nil
	case "^":
		return math.Pow(left, right), nil
	case ":":
		return left*60 + right, nil
	case "|", "or":
		return float64(toUint32(left) | toUint32(right)), nil
	case "&", "and":
		return float64(toUint32(left) & toUint32(right)), nil
	case ">>", "shr":
		return float64(toUint32(left) >> (toUint32(right) & 31)), nil
	case "<<", "shl":
		return float64(toUint32(left) << (toUint32(right) & 31)), nil
	case "=":
		return boolValue(left == right), nil
	case "#", "!=", "<>":
		return boolValue(left != right), nil
	case "<":
		return boolValue(left < right), nil
	case ">":
		return boolValue(left > right), nil
	case "<=":
		return boolValue(left <= right), nil
	case ">=":
		return boolValue(left >= right), nil
	}
	return 0, errorAt(n.pos, "unknown operator %q", n.op)
}

func evalCall(n *callNode, env Env) (float64, error) {
	switch n.name {
	case "sum", "mul":
		return evalAggregate(n, env)
	case "if":
		condition, err := Eval(n.args[0], env)
		if err != nil {
			return 0, err
		}
		if condition != 0 {
			return Eval(n.args[1], env)
		}
		return Eval(n.args[2], env)
	}

	args := make([]float64, len(n.args))
	for i, arg := range n.args {
		value, err := Eval(arg, env)
		if err != nil {
			return 0, err
		}
		args[i] = value
	}
	return applyFunction(n, args)
}

// evalAggregate implements sum() and mul(). Empty and text cells inside a
// range are skipped instead of contributing a zero, which is the whole point
// of using a range rather than a chain of operators.
func evalAggregate(n *callNode, env Env) (float64, error) {
	var values []float64
	for _, arg := range n.args {
		if r, ok := arg.(*rangeNode); ok {
			if env == nil {
				return 0, errorAt(r.pos, "references are not available here")
			}
			collected, err := env.RangeValues(r.from, r.to)
			if err != nil {
				return 0, errorAt(r.pos, "%v", err)
			}
			values = append(values, collected...)
			continue
		}
		value, err := Eval(arg, env)
		if err != nil {
			return 0, err
		}
		values = append(values, value)
	}
	if n.name == "sum" {
		total := 0.0
		for _, value := range values {
			total += value
		}
		return total, nil
	}
	if len(values) == 0 {
		return 0, nil
	}
	product := 1.0
	for _, value := range values {
		product *= value
	}
	return product, nil
}

func applyFunction(n *callNode, args []float64) (float64, error) {
	switch n.name {
	case "pi":
		return math.Pi, nil
	case "sin":
		return math.Sin(args[0]), nil
	case "cos":
		return math.Cos(args[0]), nil
	case "tan", "tg":
		return math.Tan(args[0]), nil
	case "cotan", "ctg":
		if math.Tan(args[0]) == 0 {
			return 0, errorAt(n.pos, "%s is undefined here", n.name)
		}
		return 1 / math.Tan(args[0]), nil
	case "sec":
		if math.Cos(args[0]) == 0 {
			return 0, errorAt(n.pos, "sec is undefined here")
		}
		return 1 / math.Cos(args[0]), nil
	case "cosec":
		if math.Sin(args[0]) == 0 {
			return 0, errorAt(n.pos, "cosec is undefined here")
		}
		return 1 / math.Sin(args[0]), nil
	case "arcsin", "asin":
		return checkNaN(n, math.Asin(args[0]))
	case "arccos", "acos":
		return checkNaN(n, math.Acos(args[0]))
	case "atan", "arctan":
		return math.Atan(args[0]), nil
	case "actg", "arccotan":
		return math.Pi/2 - math.Atan(args[0]), nil
	case "arcsec":
		if args[0] == 0 {
			return 0, errorAt(n.pos, "arcsec is undefined here")
		}
		return checkNaN(n, math.Acos(1/args[0]))
	case "arccosec":
		if args[0] == 0 {
			return 0, errorAt(n.pos, "arccosec is undefined here")
		}
		return checkNaN(n, math.Asin(1/args[0]))
	case "rad":
		return args[0] * math.Pi / 180, nil
	case "radg":
		return args[0] * math.Pi / 200, nil
	case "deg":
		return args[0] * 180 / math.Pi, nil
	case "grad":
		return args[0] * 200 / math.Pi, nil
	case "sh", "sinh":
		return math.Sinh(args[0]), nil
	case "ch", "cosh":
		return math.Cosh(args[0]), nil
	case "th", "tanh":
		return math.Tanh(args[0]), nil
	case "cth", "cotanh":
		if math.Tanh(args[0]) == 0 {
			return 0, errorAt(n.pos, "%s is undefined here", n.name)
		}
		return 1 / math.Tanh(args[0]), nil
	case "arch", "arccosh":
		return checkNaN(n, math.Acosh(args[0]))
	case "ash", "arcsinh":
		return math.Asinh(args[0]), nil
	case "ath", "arctanh":
		return checkNaN(n, math.Atanh(args[0]))
	case "exp":
		return math.Exp(args[0]), nil
	case "fact":
		if args[0] < 0 {
			return 0, errorAt(n.pos, "fact is undefined for negative arguments")
		}
		return checkNaN(n, math.Gamma(args[0]+1))
	case "lg":
		return checkNaN(n, math.Log10(args[0]))
	case "ln":
		return checkNaN(n, math.Log(args[0]))
	case "sqr":
		return args[0] * args[0], nil
	case "sqrt":
		return checkNaN(n, math.Sqrt(args[0]))
	case "round":
		return math.Round(args[0]), nil
	case "sign":
		switch {
		case args[0] > 0:
			return 1, nil
		case args[0] < 0:
			return -1, nil
		}
		return 0, nil
	case "abs":
		return math.Abs(args[0]), nil
	case "log":
		if args[0] <= 0 || args[0] == 1 || args[1] <= 0 {
			return 0, errorAt(n.pos, "log is undefined here")
		}
		return math.Log(args[1]) / math.Log(args[0]), nil
	case "root":
		if args[0] == 0 {
			return 0, errorAt(n.pos, "root is undefined here")
		}
		return checkNaN(n, math.Pow(args[1], 1/args[0]))
	}
	return 0, errorAt(n.pos, "unknown function %q", n.name)
}

func checkNaN(n *callNode, value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errorAt(n.pos, "%s is undefined here", n.name)
	}
	return value, nil
}

func boolValue(condition bool) float64 {
	if condition {
		return TrueValue
	}
	return 0
}

func toUint32(value float64) uint32 {
	rounded := math.Round(value)
	if math.IsNaN(rounded) || math.IsInf(rounded, 0) {
		return 0
	}
	wrapped := math.Mod(rounded, 1<<32)
	if wrapped < 0 {
		wrapped += 1 << 32
	}
	return uint32(wrapped)
}
