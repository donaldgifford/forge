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
//
// The rewriter (added in B.3) walks the v1 AST via text/template/parse
// and emits HCL for every node kind it knows. Out-of-scope nodes
// (range, with, template, break, continue) surface as
// UntranslatedHits so the author can fix them by hand.
package migratecmd
