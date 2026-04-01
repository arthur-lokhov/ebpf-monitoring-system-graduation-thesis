package filter

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TokenType represents the type of a token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenNumber
	TokenString
	TokenLBrace
	TokenRBrace
	TokenLParen
	TokenRParen
	TokenLBracket
	TokenRBracket
	TokenComma
	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenAssign
	TokenBy
	TokenWithout
)

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

// Lexer performs lexical analysis on filter expressions
type Lexer struct {
	input string
	pos   int
	ch    byte
}

// NewLexer creates a new lexer
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.pos]
	}
	l.pos++
}

func (l *Lexer) peekChar() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// NextToken returns the next token
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	tok := Token{Pos: l.pos - 1}

	switch l.ch {
	case '=':
		tok.Type = TokenAssign
		tok.Literal = "="
	case '+':
		tok.Type = TokenPlus
		tok.Literal = "+"
	case '-':
		tok.Type = TokenMinus
		tok.Literal = "-"
	case '*':
		tok.Type = TokenStar
		tok.Literal = "*"
	case '/':
		tok.Type = TokenSlash
		tok.Literal = "/"
	case ',':
		tok.Type = TokenComma
		tok.Literal = ","
	case '(':
		tok.Type = TokenLParen
		tok.Literal = "("
	case ')':
		tok.Type = TokenRParen
		tok.Literal = ")"
	case '{':
		tok.Type = TokenLBrace
		tok.Literal = "{"
	case '}':
		tok.Type = TokenRBrace
		tok.Literal = "}"
	case '[':
		tok.Type = TokenLBracket
		tok.Literal = "["
	case ']':
		tok.Type = TokenRBracket
		tok.Literal = "]"
	case 0:
		tok.Type = TokenEOF
		tok.Literal = ""
	case '"':
		tok.Type = TokenString
		tok.Literal = l.readString()
		return tok
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = l.lookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok.Type = TokenNumber
			tok.Literal = l.readNumber()
			return tok
		} else {
			tok.Type = TokenEOF
			tok.Literal = string(l.ch)
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.pos - 1
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position : l.pos-1]
}

func (l *Lexer) readNumber() string {
	position := l.pos - 1
	for isDigit(l.ch) || l.ch == '.' {
		l.readChar()
	}
	return l.input[position : l.pos-1]
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening quote
	start := l.pos - 1
	for l.ch != '"' && l.ch != 0 {
		l.readChar()
	}
	end := l.pos - 1
	l.readChar() // skip closing quote
	return l.input[start:end]
}

func (l *Lexer) lookupIdent(ident string) TokenType {
	switch strings.ToLower(ident) {
	case "by":
		return TokenBy
	case "without":
		return TokenWithout
	default:
		return TokenIdent
	}
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

// Parser parses filter expressions into AST
type Parser struct {
	lexer         *Lexer
	curToken      Token
	peekToken     Token
	currentPrecedence int
}

// NewParser creates a new parser
func NewParser(l *Lexer) *Parser {
	p := &Parser{lexer: l}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

func (p *Parser) curTokenIs(t TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t TokenType) bool {
	return p.peekToken.Type == t
}

// Parse parses the expression into an AST
func (p *Parser) Parse() (ASTNode, error) {
	if p.curTokenIs(TokenEOF) {
		return nil, fmt.Errorf("empty expression")
	}

	return p.parseExpression()
}

func (p *Parser) parseExpression() (ASTNode, error) {
	return p.parseAggregation()
}

func (p *Parser) parseAggregation() (ASTNode, error) {
	if !p.curTokenIs(TokenIdent) {
		return p.parseRangeSelector()
	}

	ident := p.curToken.Literal
	lowerIdent := strings.ToLower(ident)

	// Check for aggregation operators
	switch lowerIdent {
	case "sum", "avg", "min", "max", "count":
		return p.parseAggregationOperator(ident)
	case "rate", "irate", "increase":
		return p.parseRateFunction(ident)
	case "per_second", "per_minute", "per_hour":
		return p.parseNormalizationFunction(ident)
	case "histogram_quantile":
		return p.parseHistogramQuantile()
	case "label_join", "label_replace":
		return p.parseLabelFunction(ident)
	}

	return p.parseRangeSelector()
}

func (p *Parser) parseAggregationOperator(op string) (ASTNode, error) {
	p.nextToken() // consume operator

	if !p.curTokenIs(TokenBy) && !p.curTokenIs(TokenWithout) {
		// Simple aggregation without grouping
		expr, err := p.parseRangeSelector()
		if err != nil {
			return nil, err
		}
		return &AggregationNode{
			Operator: op,
			Expr:     expr,
		}, nil
	}

	// Aggregation with grouping
	groupingType := p.curToken.Literal
	p.nextToken() // consume by/without

	if !p.curTokenIs(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after %s", groupingType)
	}

	labels, err := p.parseLabelList()
	if err != nil {
		return nil, err
	}

	expr, err := p.parseRangeSelector()
	if err != nil {
		return nil, err
	}

	return &AggregationNode{
		Operator:     op,
		Expr:         expr,
		GroupingType: groupingType,
		GroupLabels:  labels,
	}, nil
}

func (p *Parser) parseRateFunction(funcName string) (ASTNode, error) {
	p.nextToken() // consume function name

	if !p.curTokenIs(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after %s", funcName)
	}

	expr, err := p.parseRangeSelector()
	if err != nil {
		return nil, err
	}

	return &RateNode{
		Function: funcName,
		Expr:     expr,
	}, nil
}

func (p *Parser) parseNormalizationFunction(funcName string) (ASTNode, error) {
	p.nextToken() // consume function name

	if !p.curTokenIs(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after %s", funcName)
	}

	expr, err := p.parseRangeSelector()
	if err != nil {
		return nil, err
	}

	return &NormalizationNode{
		Function: funcName,
		Expr:     expr,
	}, nil
}

func (p *Parser) parseHistogramQuantile() (ASTNode, error) {
	p.nextToken() // consume function name

	if !p.curTokenIs(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after histogram_quantile")
	}

	// Parse quantile value
	if p.peekTokenIs(TokenComma) {
		return nil, fmt.Errorf("expected quantile value")
	}
	p.nextToken()

	quantile, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid quantile value: %s", p.curToken.Literal)
	}

	if !p.peekTokenIs(TokenComma) {
		return nil, fmt.Errorf("expected ',' after quantile")
	}
	p.nextToken() // consume comma
	p.nextToken() // move to bucket expression

	bucketExpr, err := p.parseRangeSelector()
	if err != nil {
		return nil, err
	}

	return &HistogramQuantileNode{
		Quantile:   quantile,
		BucketExpr: bucketExpr,
	}, nil
}

func (p *Parser) parseLabelFunction(funcName string) (ASTNode, error) {
	p.nextToken() // consume function name

	if !p.curTokenIs(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after %s", funcName)
	}

	// Parse arguments based on function
	args := make([]string, 0)
	for !p.peekTokenIs(TokenRParen) {
		p.nextToken()
		if p.curTokenIs(TokenString) || p.curTokenIs(TokenIdent) {
			args = append(args, p.curToken.Literal)
		}
		if p.peekTokenIs(TokenComma) {
			p.nextToken() // consume comma
		}
	}

	p.nextToken() // consume RParen

	// Parse source expression
	expr, err := p.parseRangeSelector()
	if err != nil {
		return nil, err
	}

	return &LabelFunctionNode{
		Function: funcName,
		Args:     args,
		Expr:     expr,
	}, nil
}

func (p *Parser) parseRangeSelector() (ASTNode, error) {
	// Parse metric name or subexpression
	var expr ASTNode

	if p.curTokenIs(TokenLParen) {
		p.nextToken()
		subExpr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		expr = subExpr
		if !p.curTokenIs(TokenRParen) {
			return nil, fmt.Errorf("expected ')', got %s", p.curToken.Literal)
		}
		p.nextToken()
	} else if p.curTokenIs(TokenIdent) {
		expr = &MetricSelectorNode{
			Name: p.curToken.Literal,
		}
		p.nextToken()
	} else {
		return nil, fmt.Errorf("unexpected token: %s", p.curToken.Literal)
	}

	// Check for label matchers
	if p.curTokenIs(TokenLBrace) {
		selector, ok := expr.(*MetricSelectorNode)
		if !ok {
			return nil, fmt.Errorf("label matchers can only be applied to metrics")
		}

		labels, err := p.parseLabelMatchers()
		if err != nil {
			return nil, err
		}
		selector.LabelMatchers = labels
		p.nextToken() // consume RBrace
	}

	// Check for range selector
	if p.curTokenIs(TokenLBracket) {
		p.nextToken() // consume LBracket

		duration, err := p.parseDuration()
		if err != nil {
			return nil, err
		}

		if !p.curTokenIs(TokenRBracket) {
			return nil, fmt.Errorf("expected ']', got %s", p.curToken.Literal)
		}
		p.nextToken() // consume RBracket

		// Wrap in range selector
		return &RangeSelectorNode{
			Expr:     expr,
			Range:    duration,
		}, nil
	}

	return expr, nil
}

func (p *Parser) parseLabelMatchers() (map[string]string, error) {
	labels := make(map[string]string)

	for !p.curTokenIs(TokenRBrace) {
		if !p.curTokenIs(TokenIdent) {
			return nil, fmt.Errorf("expected label name, got %s", p.curToken.Literal)
		}
		labelName := p.curToken.Literal

		p.nextToken() // consume label name
		if !p.curTokenIs(TokenAssign) {
			return nil, fmt.Errorf("expected '=', got %s", p.curToken.Literal)
		}

		p.nextToken() // consume =
		if !p.curTokenIs(TokenString) {
			return nil, fmt.Errorf("expected label value, got %s", p.curToken.Literal)
		}
		labelValue := p.curToken.Literal

		labels[labelName] = labelValue

		p.nextToken() // consume value
		if p.curTokenIs(TokenComma) {
			p.nextToken()
		}
	}

	return labels, nil
}

func (p *Parser) parseLabelList() ([]string, error) {
	labels := make([]string, 0)

	if !p.curTokenIs(TokenLParen) {
		return labels, nil
	}

	for !p.peekTokenIs(TokenRParen) {
		p.nextToken()
		if p.curTokenIs(TokenIdent) {
			labels = append(labels, p.curToken.Literal)
		}
		if p.peekTokenIs(TokenComma) {
			p.nextToken() // consume comma
		}
	}
	p.nextToken() // consume RParen

	return labels, nil
}

func (p *Parser) parseDuration() (time.Duration, error) {
	if !p.curTokenIs(TokenNumber) {
		return 0, fmt.Errorf("expected number, got %s", p.curToken.Literal)
	}

	numStr := p.curToken.Literal
	p.nextToken()

	// Parse unit
	unit := time.Second
	if p.curTokenIs(TokenIdent) {
		switch p.curToken.Literal {
		case "s", "sec", "second", "seconds":
			unit = time.Second
		case "m", "min", "minute", "minutes":
			unit = time.Minute
		case "h", "hour", "hours":
			unit = time.Hour
		case "d", "day", "days":
			unit = 24 * time.Hour
		case "w", "week", "weeks":
			unit = 7 * 24 * time.Hour
		case "y", "year", "years":
			unit = 365 * 24 * time.Hour
		case "ms":
			unit = time.Millisecond
		}
		p.nextToken()
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStr)
	}

	return time.Duration(num * float64(unit)), nil
}

// ParseExpression parses a filter expression string into an AST
func ParseExpression(expr string) (ASTNode, error) {
	l := NewLexer(expr)
	p := NewParser(l)
	return p.Parse()
}
