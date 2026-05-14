// Package migratecmd implements `forge migrate templates`, the one-shot
// rewrite tool that converts v1 (Go text/template) blueprints to v2 (HCL2).
// See IMPL-0004 / DESIGN-0003 for the design.
//
// # Corpus survey (IMPL-0004 task B.1)
//
// The rewrite-rule set was derived from the patterns observed in
// github.com/donaldgifford/forge-registry on the date this package was
// authored. The corpus contained 12 .tmpl/blueprint.yaml files across
// _defaults/, go/{std,ext}/, rust/{std,esp32}/, and std/. Every v1
// expression observed was one of:
//
//   - Bare scalar field access:   {{ .project_name }}, {{ .project_owner }},
//     {{ .project_description }}
//   - Path shorthand (no dot):    {{project_name}}/  (in rename keys and
//     directory names like {{project_name}}/main.go)
//   - Path field access:          {{ .project_name }}/main.go
//
// No range, with, custom funcs, pipes, conditionals, escapes, or
// `default`/`coalesce` calls were present in the corpus. The
// DESIGN-0003 rewrite-rule table covers every pattern observed; no
// table additions were needed.
package migratecmd

import (
	"fmt"
	"regexp"
	"strings"
	"text/template/parse"
)

// pathShorthandPattern matches the v1 `{{name}}` path shorthand (single
// identifier, no leading dot, no pipe, no parens). The migrator
// normalises these to `{{ .name }}` before parsing so the v1 parser
// doesn't reject them as undefined function references. The walker then
// emits `${name}` from the resulting FieldNode.
var pathShorthandPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// pathDottedPattern matches the v1 `{{ .name }}` dotted form used in path
// segments (with optional whitespace). Used by the directory renamer so
// authors who wrote `{{ .project_name }}` as a directory name get the same
// `${name}` treatment as the shorthand form.
var pathDottedPattern = regexp.MustCompile(`\{\{\s*\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// knownFuncs declares the v1 forge custom functions plus stdlib
// comparators so the parser doesn't reject sources that use them. The
// function bodies don't matter — parse.Parse only checks that the names
// exist.
var knownFuncs = map[string]any{
	// v1 custom funcs (internal/template/funcs.go).
	"snakeCase":  func() {},
	"camelCase":  func() {},
	"pascalCase": func() {},
	"kebabCase":  func() {},
	"upper":      func() {},
	"lower":      func() {},
	"title":      func() {},
	"replace":    func() {},
	"trimPrefix": func() {},
	"trimSuffix": func() {},
	"now":        func() {},
	"env":        func() {},
	"default":    func() {},
	// stdlib comparators recognised inside `if` conditions.
	"eq":  func() {},
	"ne":  func() {},
	"not": func() {},
	"and": func() {},
	"or":  func() {},
}

// RewriteTemplate parses src as a v1 Go text/template body and returns
// the equivalent HCL2 source. Untranslatable constructs (range, with,
// template, break, continue) are surfaced as UntranslatedHits with
// source-line info so the author can fix them by hand; the offending
// region is emitted verbatim into the output as a best-effort fallback.
//
// Every `{{ .X }}` is translated to `${X}` regardless of whether `X`
// is a declared variable — appropriate for the v0.2.x → v0.3.x cutover
// where every `{{ }}` in a .tmpl file was forge syntax. For v0.4.x+
// registries that mix forge variables with downstream-tool `{{ }}`
// syntax (goreleaser, Helm), use RewriteTemplateScoped instead.
//
// The function is pure: it never touches the filesystem or network.
func RewriteTemplate(name, src string) (string, []UntranslatedHit, error) {
	return rewriteTemplate(name, src, nil)
}

// RewriteTemplateScoped is the variable-aware variant of RewriteTemplate.
// Simple `{{ .name }}` actions whose name isn't in declared are emitted
// verbatim — they're treated as downstream-tool syntax (goreleaser's
// `{{ .ProjectName }}`, Helm's `{{ .Values.replicas }}`, etc.) rather
// than v1 forge references. Chained field access (`{{ .a.b }}`) is
// always emitted verbatim regardless of the scope, since forge
// variables are single-level. Function calls and pipes always migrate
// because they have no downstream-tool analogue.
//
// Passing a nil declared slice yields the same behavior as
// RewriteTemplate (translate every field).
func RewriteTemplateScoped(name, src string, declared []string) (string, []UntranslatedHit, error) {
	var scope map[string]struct{}

	if declared != nil {
		scope = make(map[string]struct{}, len(declared))
		for _, v := range declared {
			scope[v] = struct{}{}
		}
	}

	return rewriteTemplate(name, src, scope)
}

func rewriteTemplate(name, src string, scope map[string]struct{}) (string, []UntranslatedHit, error) {
	normalised := normalisePathShorthand(src)

	tree, err := parse.New(name).Parse(normalised, "{{", "}}", map[string]*parse.Tree{}, knownFuncs)
	if err != nil {
		return "", nil, fmt.Errorf("parsing v1 template %q: %w", name, err)
	}

	var w *walker
	if scope == nil {
		w = newWalker(name, normalised)
	} else {
		w = newWalkerScoped(name, normalised, scope)
	}

	w.walkList(tree.Root)

	return w.out.String(), w.hits, nil
}

// RewriteCondition translates a v1 condition.when expression into the
// bare HCL bool expression form expected by the v2 renderer's
// EvaluateBool. Unlike RewriteTemplate, the result is *not* wrapped in
// ${ … } — `condition.when:` is evaluated as an expression directly,
// not as a template body.
//
// Falls through to RewriteTemplate when the input doesn't look like a
// single-action expression, so condition.when fields that contain
// arbitrary template content still migrate.
func RewriteCondition(name, src string) (string, []UntranslatedHit, error) {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" {
		return "", nil, nil
	}

	normalised := normalisePathShorthand(trimmed)

	tree, err := parse.New(name).Parse(normalised, "{{", "}}", map[string]*parse.Tree{}, knownFuncs)
	if err != nil {
		return "", nil, fmt.Errorf("parsing v1 condition %q: %w", name, err)
	}

	if tree.Root == nil || len(tree.Root.Nodes) != 1 {
		return RewriteTemplate(name, src)
	}

	action, ok := tree.Root.Nodes[0].(*parse.ActionNode)
	if !ok {
		return RewriteTemplate(name, src)
	}

	w := newWalker(name, normalised)

	cond, ok := w.translateCondition(action.Pipe)
	if !ok {
		return RewriteTemplate(name, src)
	}

	return cond, w.hits, nil
}

// templateKeywords are control-flow words the v1 parser recognises as
// directives, not identifiers. The path-shorthand normaliser must leave
// them alone so `{{ end }}`, `{{ else }}` etc. keep their meaning.
var templateKeywords = map[string]struct{}{
	"if":       {},
	"else":     {},
	"end":      {},
	"range":    {},
	"with":     {},
	"template": {},
	"block":    {},
	"define":   {},
	"break":    {},
	"continue": {},
	"true":     {},
	"false":    {},
	"nil":      {},
}

// normalisePathShorthand rewrites `{{name}}` to `{{ .name }}` for any
// identifier that isn't a known function, comparator, or template
// keyword. This lets the parser accept the v1 path-templating shorthand
// that some blueprints use in directory names, rename keys, and the
// like.
func normalisePathShorthand(src string) string {
	return pathShorthandPattern.ReplaceAllStringFunc(src, func(match string) string {
		ident := strings.TrimSpace(match[2 : len(match)-2])
		if _, isFunc := knownFuncs[ident]; isFunc {
			return match
		}

		if _, isKeyword := templateKeywords[ident]; isKeyword {
			return match
		}

		return "{{ ." + ident + " }}"
	})
}

type walker struct {
	name string
	src  string
	out  strings.Builder
	hits []UntranslatedHit
	// allowedVars scopes single-field `{{ .X }}` rewrites to declared
	// forge variables. A nil map means "translate every field" — the
	// legacy v0.2.x → v0.3.x behavior where every `{{ }}` was forge
	// syntax. A non-nil map means: simple `{{ .name }}` actions whose
	// name is absent from the map are emitted verbatim (they're
	// downstream-tool syntax like goreleaser's `{{ .ProjectName }}` or
	// Helm's `{{ .Values.replicas }}`, not v1 forge references).
	allowedVars map[string]struct{}
}

func newWalker(name, src string) *walker {
	return &walker{name: name, src: src}
}

func newWalkerScoped(name, src string, allowed map[string]struct{}) *walker {
	return &walker{name: name, src: src, allowedVars: allowed}
}

func (w *walker) walkList(list *parse.ListNode) {
	if list == nil {
		return
	}

	for _, node := range list.Nodes {
		w.walkNode(node)
	}
}

func (w *walker) walkNode(node parse.Node) {
	switch n := node.(type) {
	case *parse.TextNode:
		w.out.Write(n.Text)
	case *parse.ActionNode:
		w.walkAction(n)
	case *parse.IfNode:
		w.walkIf(n)
	case *parse.RangeNode:
		w.recordUntranslated(n.Pos, "range block")
		w.out.WriteString(verbatim(w.src, n.Pos, len(n.String())))
	case *parse.WithNode:
		w.recordUntranslated(n.Pos, "with block")
		w.out.WriteString(verbatim(w.src, n.Pos, len(n.String())))
	case *parse.TemplateNode:
		w.recordUntranslated(n.Pos, "template invocation")
		w.out.WriteString(verbatim(w.src, n.Pos, len(n.String())))
	case *parse.BreakNode:
		w.recordUntranslated(n.Pos, "break statement")
	case *parse.ContinueNode:
		w.recordUntranslated(n.Pos, "continue statement")
	default:
		w.recordUntranslated(node.Position(), fmt.Sprintf("unsupported node %T", node))
		w.out.WriteString(node.String())
	}
}

// walkAction translates a `{{ … }}` action node into a `${ … }` HCL
// interpolation. The wrapped PipeNode is decomposed into nested function
// calls when more than one command is present.
//
// When the walker carries a non-nil allowedVars scope, simple
// `{{ .name }}` actions whose name isn't in the scope (including any
// chained `{{ .a.b }}` reference, since forge variables never chain)
// are emitted verbatim — they're downstream-tool syntax (goreleaser's
// `{{ .ProjectName }}`, Helm's `{{ .Values.replicas }}`, etc.) that
// must pass through the renderer untouched.
func (w *walker) walkAction(n *parse.ActionNode) {
	if w.allowedVars != nil && simpleField(n) && !w.fieldIsAllowed(n) {
		w.out.WriteString(w.actionSource(n))

		return
	}

	expr, ok := w.translatePipe(n.Pipe)
	if !ok {
		w.out.WriteString(n.String())

		return
	}

	w.out.WriteString("${")
	w.out.WriteString(expr)
	w.out.WriteString("}")
}

// actionSource returns the original source text for an action,
// preserving the author's whitespace inside the `{{ }}`. parse.Node's
// String() method rebuilds from the parse tree and strips internal
// spacing — that's irrelevant for actions we're translating, but
// matters for actions we're passing through verbatim because they're
// destined for a downstream tool (Helm, goreleaser) that may be
// whitespace-sensitive in expression contexts.
//
// ActionNode.Pos in the Go parse package points at the action body
// (the `.` in `{{ .X }}`), not at the opening `{{` delimiter, so we
// scan backwards to find the `{{` before slicing.
func (w *walker) actionSource(n *parse.ActionNode) string {
	pos := int(n.Pos)
	if pos >= len(w.src) {
		return n.String()
	}

	start := strings.LastIndex(w.src[:pos], "{{")
	if start < 0 {
		return n.String()
	}

	i := strings.Index(w.src[start:], "}}")
	if i < 0 {
		return n.String()
	}

	return w.src[start : start+i+2]
}

// simpleField reports whether the action is shaped `{{ .X }}` or
// `{{ .X.Y... }}` — a single command with a single FieldNode argument.
// Function calls (`{{ snakeCase .x }}`), pipes (`{{ .x | upper }}`),
// and multi-command actions return false.
func simpleField(n *parse.ActionNode) bool {
	if n == nil || n.Pipe == nil || len(n.Pipe.Cmds) != 1 {
		return false
	}

	cmd := n.Pipe.Cmds[0]
	if len(cmd.Args) != 1 {
		return false
	}

	_, ok := cmd.Args[0].(*parse.FieldNode)

	return ok
}

// fieldIsAllowed reports whether the root identifier of a simple-field
// action is in the allowedVars scope. Callers must verify the action
// is a simple-field action first (via simpleField). Chained field
// references (`{{ .a.b }}`) always return false — forge variables are
// single-level, so any chain is downstream syntax.
func (w *walker) fieldIsAllowed(n *parse.ActionNode) bool {
	field, ok := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
	if !ok || len(field.Ident) != 1 {
		return false
	}

	_, allowed := w.allowedVars[field.Ident[0]]

	return allowed
}

// walkIf translates `{{ if cond }} … {{ else }} … {{ end }}` to the
// HCL `%{ if cond ~} … %{ else ~} … %{ endif ~}` directive form. The
// trailing tilde strips one trailing newline so the rendered output
// matches text/template whitespace behavior reasonably well.
func (w *walker) walkIf(n *parse.IfNode) {
	cond, ok := w.translateCondition(n.Pipe)
	if !ok {
		w.recordUntranslated(n.Pos, "complex if condition")
		w.out.WriteString(n.String())

		return
	}

	w.out.WriteString("%{ if ")
	w.out.WriteString(cond)
	w.out.WriteString(" ~}")
	w.walkList(n.List)

	if n.ElseList != nil {
		w.out.WriteString("%{ else ~}")
		w.walkList(n.ElseList)
	}

	w.out.WriteString("%{ endif ~}")
}

// translatePipe converts a v1 PipeNode into an HCL2 expression string.
// Single-command pipes become a single identifier, function call, or
// literal; multi-command pipes nest into function calls per the
// "piped value is the last argument" rule, with `default` rewritten as
// `coalesce` per DESIGN-0003.
func (w *walker) translatePipe(p *parse.PipeNode) (string, bool) {
	if p == nil || len(p.Cmds) == 0 {
		return "", false
	}

	// Walk left-to-right; subsequent commands wrap the previous result.
	cur, ok := w.translateCommand(p.Cmds[0], "")
	if !ok {
		return "", false
	}

	for _, cmd := range p.Cmds[1:] {
		next, ok := w.translateCommand(cmd, cur)
		if !ok {
			return "", false
		}

		cur = next
	}

	return cur, true
}

// translateCommand translates one CommandNode (a sequence of arguments
// inside an action) into an HCL2 expression. When piped is non-empty it
// is inserted as the *first* argument of any function call — that
// matches both v1's piped-value-is-last-arg semantics and the cty/stdlib
// convention where the subject string comes first.
func (w *walker) translateCommand(cmd *parse.CommandNode, piped string) (string, bool) {
	if cmd == nil || len(cmd.Args) == 0 {
		return "", false
	}

	first := cmd.Args[0]

	// `default` becomes `coalesce` per DESIGN-0003.
	if id, ok := first.(*parse.IdentifierNode); ok && id.Ident == "default" {
		args := w.translateArgs(cmd.Args[1:])
		if piped != "" {
			args = append([]string{piped}, args...)
		}
		// Reorder for the positional v1 form `{{ default fb x }}`:
		// the v1 sig is (def, val); coalesce wants (val, def). When
		// not piped and exactly two args, swap to coalesce(val, def).
		if piped == "" && len(args) == 2 {
			args[0], args[1] = args[1], args[0]
		}

		return "coalesce(" + strings.Join(args, ", ") + ")", true
	}

	if id, ok := first.(*parse.IdentifierNode); ok {
		args := w.translateArgs(cmd.Args[1:])
		if piped != "" {
			args = append([]string{piped}, args...)
		}

		return id.Ident + "(" + strings.Join(args, ", ") + ")", true
	}

	// Single non-identifier argument: a literal, field, or variable.
	if len(cmd.Args) == 1 && piped == "" {
		return w.translateArg(first)
	}

	return "", false
}

func (w *walker) translateArgs(args []parse.Node) []string {
	out := make([]string, 0, len(args))

	for _, a := range args {
		s, ok := w.translateArg(a)
		if !ok {
			continue
		}

		out = append(out, s)
	}

	return out
}

// translateArg renders a single CommandNode argument as an HCL2
// expression. Identifiers stay as bare identifiers; FieldNodes drop the
// leading dot and join chained accesses with `.`; string/number/bool
// literals translate verbatim.
func (w *walker) translateArg(a parse.Node) (string, bool) {
	switch x := a.(type) {
	case *parse.FieldNode:
		return strings.Join(x.Ident, "."), true
	case *parse.IdentifierNode:
		return x.Ident, true
	case *parse.VariableNode:
		// `$x` in templates → `x` in HCL.
		return strings.TrimPrefix(strings.Join(x.Ident, "."), "$"), true
	case *parse.StringNode:
		return x.Quoted, true
	case *parse.NumberNode:
		return x.Text, true
	case *parse.BoolNode:
		if x.True {
			return "true", true
		}

		return "false", true
	case *parse.PipeNode:
		// Sub-pipes are rare but legal (e.g. inside if conditions).
		return w.translatePipe(x)
	default:
		return "", false
	}
}

// translateCondition produces a bool expression suitable for an HCL2
// `%{ if … ~}` directive. Recognises the v1 comparator helpers
// `eq` / `ne` / `not` and rewrites them with native HCL operators per
// DESIGN-0003.
func (w *walker) translateCondition(p *parse.PipeNode) (string, bool) {
	if p == nil || len(p.Cmds) != 1 {
		// Multi-command condition pipes are unusual and not supported
		// by the migrator; let the generic path attempt them.
		return w.translatePipe(p)
	}

	cmd := p.Cmds[0]
	if len(cmd.Args) == 0 {
		return "", false
	}

	id, ok := cmd.Args[0].(*parse.IdentifierNode)
	if !ok {
		return w.translatePipe(p)
	}

	rest := w.translateArgs(cmd.Args[1:])

	switch id.Ident {
	case "eq":
		if len(rest) == 2 {
			return rest[0] + " == " + rest[1], true
		}
	case "ne":
		if len(rest) == 2 {
			return rest[0] + " != " + rest[1], true
		}
	case "not":
		if len(rest) == 1 {
			return "!" + rest[0], true
		}
	}

	return w.translatePipe(p)
}

func (w *walker) recordUntranslated(pos parse.Pos, reason string) {
	line := lineOf(w.src, int(pos))
	snippet := snippetAt(w.src, int(pos))

	w.hits = append(w.hits, UntranslatedHit{
		File:    w.name,
		Line:    line,
		Snippet: snippet,
		Reason:  reason,
	})
}

// lineOf returns the 1-based line number of byte offset off in src.
func lineOf(src string, off int) int {
	if off > len(src) {
		off = len(src)
	}

	return strings.Count(src[:off], "\n") + 1
}

// snippetAt returns the line of src starting at byte offset off, trimmed
// to a single line, used for diagnostic display.
func snippetAt(src string, off int) string {
	if off >= len(src) {
		return ""
	}

	end := strings.IndexByte(src[off:], '\n')
	if end < 0 {
		return strings.TrimSpace(src[off:])
	}

	return strings.TrimSpace(src[off : off+end])
}

// verbatim returns approximately n bytes of src starting at off, used as
// the fallback emission for untranslatable nodes so authors can find the
// offending region in the v2 file.
func verbatim(src string, pos parse.Pos, n int) string {
	off := int(pos)
	if off >= len(src) {
		return ""
	}

	if off+n > len(src) {
		n = len(src) - off
	}

	return src[off : off+n]
}
