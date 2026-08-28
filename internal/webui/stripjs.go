package webui

import "bytes"

// stripJS removes comments from src, a full JavaScript source file or an inline <script>
// element's body.
//
// # Why this cannot be a regex
//
// A regex sees "//" and "/*" as strings, not as syntax, so it cannot tell a comment from
// the same two characters inside a string literal, a template literal, or a regex literal
// -- and app.js has all three. analytics.js's GA loader has a plain string with a comment
// delimiter inside it ('https://www.googletagmanager.com/...', which a naive strip would
// truncate mid-string), and app.js has three regex literals
// (/^replace-(\d+)$/.exec(hash) and its two siblings) whose own '/' characters a
// division-vs-regex-blind scanner cannot distinguish from a divide operator, let alone
// from the start of a comment inside one. This is a real, if small, tokenizer: it tracks
// enough JS grammar to know when a '/' divides two numbers, opens a regex literal, or
// starts a comment, and it walks template literals recursively so a `${ ... }`
// substitution containing its own strings, comments and nested templates is scanned
// correctly rather than by counting backticks.
//
// # What it does not attempt
//
// It is not a parser. It does not build an AST, does not know what a token MEANS, and
// does not reformat, rename or reorder anything -- only comment bytes are ever omitted
// from the output. The one exception is a `/*!` license comment or a sourceMappingURL /
// sourceURL directive comment, either // or /*-style, which are copied through verbatim:
// removing an attribution notice or breaking devtools' source-map lookup would be a
// behaviour change, not a size reduction, and this project ships no build step that could
// regenerate either.
func stripJS(src []byte) []byte {
	s := &jsScanner{src: src, out: &bytes.Buffer{}}
	s.out.Grow(len(src))
	s.scan(false)
	return s.out.Bytes()
}

// jsScanner walks src once, left to right, copying every byte to out except comments.
//
// prevValue is the whole of the regex-vs-division heuristic: it is true when the last
// significant token scanned was something a value could follow a '/' after (an
// identifier, a literal, or a closing ')' or ']'), which makes the next bare '/' a divide
// operator. It is false after an operator, an opening bracket, a keyword like `return` or
// `typeof`, or a block's closing '}' -- contexts where JavaScript's own grammar only
// permits an expression to start, so a '/' there begins a regex literal. This is the same
// rule real JS lexers use.
//
// A ')' is not simply "value" the way ']' is: `foo(x)/2` divides, but `if(x) /re/.test(y)`
// does not -- the difference is not the paren, it is what OPENED it. parenStack carries
// that: pushed true when the '(' immediately follows if/while/for/with (a CONTROL paren,
// whose ')' starts a fresh statement, so regex may follow), false otherwise (a call or a
// grouping expression, whose ')' is a value). Getting this wrong the OTHER way -- treating
// a call's ')' as regex-allowed -- is not a risk here: it would only turn a division into
// a mis-scanned "regex" starting mid-expression, which is exactly the class of breakage
// TestStripJSOnlyRemovesComments and TestStripJSPassesNodeCheck exist to catch, run against
// every file this project ships.
//
// The one gap left is `}`, ambiguous between closing a block (regex may follow) and
// closing an object literal used as a value (division may follow) without a full parse --
// resolving it the way parenStack resolves ')' would need to know what opened the '{' too,
// which needs a parse this scanner does not do. This project's own JS -- verified by the
// node --check and token-preservation tests beside this file -- never writes the second
// form immediately followed by '/', so '}' is always treated as regex-allowed.
type jsScanner struct {
	src       []byte
	i         int
	out       *bytes.Buffer
	prevValue bool
	lastWord  string // the most recently scanned identifier/keyword; see parenStack
	// parenStack records, for each currently-open '(', whether it followed if/while/for/
	// with -- a CONTROL paren, whose matching ')' starts a statement (so a '/' right
	// after it may be a regex: `if(x) /re/.test(y)`), as opposed to a call or a grouping
	// expression, whose ')' is a value (so a '/' right after it divides: `foo(x)/2`).
	// Both are "the last thing was ')'" and are indistinguishable without this.
	parenStack []bool
}

// scan consumes tokens from s.i onward. If stopAtBrace is true, s.i is assumed to be
// positioned just after an opening '{' the caller already wrote to out; scan copies
// tokens until it reaches that brace's matching '}', writes it, and returns -- used both
// for an ordinary block/object and for a template literal's `${ ... }` substitution, which
// is exactly the same grammar once the leading `${` has been written. If stopAtBrace is
// false, scan runs to end of input; a stray top-level '}' (malformed input) is copied
// through rather than treated as a signal to stop, since nothing opened it.
func (s *jsScanner) scan(stopAtBrace bool) {
	for s.i < len(s.src) {
		c := s.src[s.i]
		switch {
		case c == '}':
			s.out.WriteByte(c)
			s.i++
			s.prevValue = false
			if stopAtBrace {
				return
			}
		case c == '{':
			s.openBrace()
		case c == '`':
			s.openTemplate()
		case isJSQuote(c):
			s.scanString(c)
			s.prevValue = true
		case c == '/':
			s.handleSlash()
		case isJSIdentStart(c):
			s.scanKeywordOrIdentifier()
		case isJSDigit(c):
			s.scanNumber()
			s.prevValue = true
		case c == '(':
			s.openParen()
		case c == ')':
			s.closeParen()
		case c == ']':
			s.out.WriteByte(c)
			s.i++
			s.prevValue = true
		default:
			s.out.WriteByte(c)
			s.i++
			s.prevValue = s.prevValue && isJSSpace(c) // unchanged on whitespace, else false
		}
	}
}

// peek returns the byte after s.i, or 0 at end of input -- 0 matches no comparison this
// scanner makes, so callers can write a plain "==" without a separate bounds check.
func (s *jsScanner) peek() byte {
	if s.i+1 >= len(s.src) {
		return 0
	}
	return s.src[s.i+1]
}

// openBrace handles a '{' encountered in normal JS: a block or an object literal, or a
// template's `${`'s brace (written by openTemplate, not here). Regex is allowed to follow
// immediately (an empty block/object's own content starts fresh).
func (s *jsScanner) openBrace() {
	s.out.WriteByte('{')
	s.i++
	s.prevValue = false
	s.scan(true)
}

// openTemplate handles a '`' encountered in normal JS: the start of a template literal.
func (s *jsScanner) openTemplate() {
	s.out.WriteByte('`')
	s.i++
	s.scanTemplateText()
	s.prevValue = true
}

// divideOrRegex handles a bare '/' that is neither "//" nor "/*": jsScanner's own doc
// comment is the reasoning behind prevValue, which is the entire decision here.
func (s *jsScanner) divideOrRegex() {
	if s.prevValue {
		s.out.WriteByte('/')
		s.i++
		s.prevValue = false
		return
	}
	s.scanRegex()
	s.prevValue = true
}

// handleSlash handles a '/' encountered in normal JS: the start of a line comment
// ("//"), a block comment ("/*"), or -- anything else -- divideOrRegex's own decision
// between a division operator and a regex literal.
func (s *jsScanner) handleSlash() {
	switch s.peek() {
	case '/':
		s.skipLineComment()
	case '*':
		s.skipBlockComment()
	default:
		s.divideOrRegex()
	}
}

// scanKeywordOrIdentifier handles an identifier or keyword encountered in normal JS,
// copying it through and recording it (in prevValue and lastWord) for whatever token
// comes next -- see jsRegexAllowedKeyword and jsScanner.parenStack.
func (s *jsScanner) scanKeywordOrIdentifier() {
	word := s.scanIdentifier()
	s.prevValue = !jsRegexAllowedKeyword[word]
	s.lastWord = word
}

// openParen handles a '(' encountered in normal JS; see jsScanner.parenStack for why it
// matters what lastWord was.
func (s *jsScanner) openParen() {
	s.out.WriteByte('(')
	s.i++
	s.parenStack = append(s.parenStack, jsControlParenKeyword[s.lastWord])
	s.prevValue = false
}

// closeParen handles a ')' encountered in normal JS, popping the matching openParen's
// verdict off parenStack. An empty stack (malformed input: an unmatched ')') is treated as
// a non-control paren, the same "value" default a stray ']' or '}' gets elsewhere in scan.
func (s *jsScanner) closeParen() {
	s.out.WriteByte(')')
	s.i++
	control := false
	if depth := len(s.parenStack); depth > 0 {
		control = s.parenStack[depth-1]
		s.parenStack = s.parenStack[:depth-1]
	}
	s.prevValue = !control
}

// scanTemplateText consumes the literal text of a template literal, starting just after
// its opening backtick, copying everything through unexamined -- a "//" or "/*" inside
// template text is literal content, never a comment -- except a `${`, which opens a
// substitution scanned as ordinary JS via scan(true), and a backslash escape, consumed as
// a pair so an escaped backtick or "${" does not end the literal or open a substitution
// early. It returns once the literal's closing backtick is written; an unterminated
// literal (malformed input) runs to end of file.
func (s *jsScanner) scanTemplateText() {
	n := len(s.src)
	for s.i < n {
		c := s.src[s.i]
		switch {
		case c == '\\' && s.i+1 < n:
			s.out.Write(s.src[s.i : s.i+2])
			s.i += 2
		case c == '`':
			s.out.WriteByte(c)
			s.i++
			return
		case c == '$' && s.i+1 < n && s.src[s.i+1] == '{':
			s.out.Write(s.src[s.i : s.i+2])
			s.i += 2
			s.prevValue = false
			s.scan(true)
		default:
			s.out.WriteByte(c)
			s.i++
		}
	}
}

// scanString copies a '...' or "..." string literal through unchanged, honouring
// backslash escapes so an escaped quote does not end it early. An unterminated string
// (malformed input) runs to end of file.
func (s *jsScanner) scanString(quote byte) {
	n := len(s.src)
	s.out.WriteByte(quote)
	s.i++
	for s.i < n {
		c := s.src[s.i]
		if c == '\\' && s.i+1 < n {
			s.out.Write(s.src[s.i : s.i+2])
			s.i += 2
			continue
		}
		s.out.WriteByte(c)
		s.i++
		if c == quote {
			return
		}
		if c == '\n' {
			return // unterminated (malformed input) -- a string literal cannot hold a raw newline
		}
	}
}

// scanRegex copies a /pattern/flags regex literal through unchanged, called only when
// prevValue says a '/' here opens a regex rather than divides. It tracks character-class
// brackets, inside which an unescaped '/' does not close the literal (`/[/]/` is a valid
// regex matching a single "/"), and honours backslash escapes everywhere else. An
// unescaped newline before the closing '/' means the input was never a valid regex
// literal (a genuine syntax error, or prevValue guessed wrong); scanRegex stops there
// without consuming the newline, the same conservative bail every other scanner here uses
// on malformed input.
func (s *jsScanner) scanRegex() {
	n := len(s.src)
	start := s.i
	s.i++
	inClass := false
	for s.i < n {
		c := s.src[s.i]
		switch {
		case c == '\\' && s.i+1 < n:
			s.i += 2
			continue
		case c == '\n':
			s.out.Write(s.src[start:s.i])
			return
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '/' && !inClass:
			s.i++
			for s.i < n && isJSIdentPart(s.src[s.i]) {
				s.i++ // flags: g, i, m, s, u, y, d
			}
			s.out.Write(s.src[start:s.i])
			return
		}
		s.i++
	}
	s.out.Write(s.src[start:s.i])
}

// scanIdentifier copies an identifier or keyword through unchanged and returns its text,
// so the caller can decide whether it is one of the keywords after which a regex may
// start. ASCII only: this project's own JS identifiers are ASCII, and a non-ASCII
// identifier byte falls through to scan's default case, which copies it unchanged too --
// only the regex-vs-division guess for whatever follows could be affected, not the bytes
// written.
func (s *jsScanner) scanIdentifier() string {
	start := s.i
	n := len(s.src)
	for s.i < n && isJSIdentPart(s.src[s.i]) {
		s.i++
	}
	word := s.src[start:s.i]
	s.out.Write(word)
	return string(word)
}

// scanNumber copies a numeric literal through unchanged. It does not need to be exact --
// digits, at most one '.', and a signed exponent cover every number this project's JS
// writes, and a boundary it gets wrong (a hex prefix, a BigInt 'n' suffix) simply falls
// through to be copied one byte at a time by scan's other cases, with the same bytes in
// the same order in the output either way. All that depends on getting the boundary right
// is prevValue, which scan already sets to true for a number regardless of how much of it
// this consumed.
func (s *jsScanner) scanNumber() {
	start := s.i
	s.consumeDigitRun()
	if s.i < len(s.src) && s.src[s.i] == '.' {
		s.i++
		s.consumeDigitRun()
	}
	s.consumeExponent()
	s.out.Write(s.src[start:s.i])
}

// consumeDigitRun advances s.i past a (possibly empty) run of ASCII digits.
func (s *jsScanner) consumeDigitRun() {
	for s.i < len(s.src) && isJSDigit(s.src[s.i]) {
		s.i++
	}
}

// consumeExponent advances s.i past a signed exponent ("e10", "E+5", "e-3") if s.i is
// positioned at the start of one; otherwise s.i is left unchanged. Only scanNumber calls
// this, once, after the mantissa -- a scan of "1e" with no following digit is correctly
// left as the number "1" followed by the separate identifier "e", the same non-exact-but-
// harmless boundary scanNumber's own doc comment already accepts.
func (s *jsScanner) consumeExponent() {
	if s.i >= len(s.src) || (s.src[s.i] != 'e' && s.src[s.i] != 'E') {
		return
	}
	j := s.i + 1
	if j < len(s.src) && (s.src[j] == '+' || s.src[j] == '-') {
		j++
	}
	if j >= len(s.src) || !isJSDigit(s.src[j]) {
		return
	}
	s.i = j
	s.consumeDigitRun()
}

// skipLineComment omits a // comment, up to but not including the terminating newline (so
// the newline itself is still copied through by scan's next iteration -- comments are not
// significant to automatic semicolon insertion, but a deleted line terminator can be, and
// this way the question never comes up). A `//!`-style bang is not a recognised license
// marker for line comments (only the block form `/*!` is, matching common convention), but
// a sourceMappingURL or sourceURL directive -- the standard way a source map or a debugger
// display name is attached to a file -- is preserved either way.
func (s *jsScanner) skipLineComment() {
	n := len(s.src)
	start := s.i
	s.i += 2
	for s.i < n && s.src[s.i] != '\n' {
		s.i++
	}
	if isPreservedJSDirective(s.src[start+2 : s.i]) {
		s.out.Write(s.src[start:s.i])
	}
}

// skipBlockComment omits a /* */ comment, except a `/*!` license block or a
// sourceMappingURL/sourceURL directive, either of which is preserved verbatim. An
// unterminated comment (malformed input) runs to end of file.
func (s *jsScanner) skipBlockComment() {
	n := len(s.src)
	start := s.i
	var end int
	if rel := bytes.Index(s.src[s.i+2:], []byte("*/")); rel >= 0 {
		end = s.i + 2 + rel + 2
	} else {
		end = n
	}
	body := s.src[start+2 : end] // includes the trailing "*/"; irrelevant to a prefix check
	if (len(body) > 0 && body[0] == '!') || isPreservedJSDirective(body) {
		s.out.Write(s.src[start:end])
	}
	s.i = end
}

// isPreservedJSDirective reports whether a comment's text (with its leading // or /* and
// trailing */ already removed) is a sourceMappingURL or sourceURL directive. Both the
// modern "#" and the legacy "@" forms are recognised; neither appears in this project's
// shipped JS today; this exists for the day a dependency or a future debug build adds one.
func isPreservedJSDirective(body []byte) bool {
	body = bytes.TrimLeft(body, " \t")
	for _, marker := range [][]byte{
		[]byte("# sourceMappingURL="), []byte("@ sourceMappingURL="),
		[]byte("#sourceMappingURL="), []byte("@sourceMappingURL="),
		[]byte("# sourceURL="), []byte("@ sourceURL="),
		[]byte("#sourceURL="), []byte("@sourceURL="),
	} {
		if bytes.HasPrefix(body, marker) {
			return true
		}
	}
	return false
}

func isJSIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isJSIdentPart(c byte) bool {
	return isJSIdentStart(c) || (c >= '0' && c <= '9')
}

func isJSSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

func isJSDigit(c byte) bool { return c >= '0' && c <= '9' }

func isJSQuote(c byte) bool { return c == '"' || c == '\'' }

// jsRegexAllowedKeyword is every keyword after which a bare '/' starts a regex literal
// rather than dividing -- because the grammar only permits an expression to start there,
// the same reasoning jsScanner's own doc comment gives for punctuation. An identifier not
// in this set (a variable name, but also `true`, `false`, `null`, `this`, `super`,
// `undefined`) is a value, so a '/' following it divides.
var jsRegexAllowedKeyword = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "export": true, "extends": true, "finally": true, "for": true,
	"from": true, "function": true, "if": true, "import": true, "in": true,
	"instanceof": true, "let": true, "new": true, "of": true, "return": true,
	"switch": true, "throw": true, "try": true, "typeof": true, "var": true,
	"void": true, "while": true, "with": true, "yield": true, "await": true,
}

// jsControlParenKeyword is which keyword, immediately before a '(', makes it a CONTROL
// paren rather than a call or a grouping expression -- see jsScanner.parenStack.
var jsControlParenKeyword = map[string]bool{"if": true, "while": true, "for": true, "with": true}
