package core

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// captureLiteralRe finds values a captured step most likely should have
// templated: URLs, dates and numbers of two or more digits.
var captureLiteralRe = regexp.MustCompile(`https?://[^\s"'<>)]+|\b\d{4}-\d{2}-\d{2}\b|\b\d{2,}\b`)

// substituteVars lifts each var's literal out of every templatable field
// as {{name}}, longest literal first so a value that contains another
// wins. A var whose literal appears nowhere is still declared, with a
// warning.
func (c *capture) substituteVars(steps []RecipeStep, vars map[string]string) {
	names := varsByLiteralLength(vars)
	hits := map[string]bool{}
	for i := range steps {
		walkStepStrings(&steps[i], func(p *string) {
			for _, name := range names {
				out, n := replaceLiteral(*p, vars[name], "{{"+name+"}}")
				if n > 0 {
					hits[name] = true
					*p = out
				}
			}
		})
	}
	for _, name := range names {
		if !hits[name] {
			c.warnf("var %s: value %q does not appear in any step; declared anyway", name, vars[name])
		}
	}
}

// varsByLiteralLength orders var names by descending literal length, then
// name; vars with an empty literal are left out (nothing to replace).
func varsByLiteralLength(vars map[string]string) []string {
	names := make([]string, 0, len(vars))
	for name, value := range vars {
		if value != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if len(vars[names[i]]) != len(vars[names[j]]) {
			return len(vars[names[i]]) > len(vars[names[j]])
		}
		return names[i] < names[j]
	})
	return names
}

// replaceLiteral replaces whole-word occurrences of literal in s that lie
// outside a {{...}} placeholder, so a value never lands inside a
// reference substituted earlier. It returns the count replaced.
func replaceLiteral(s, literal, repl string) (string, int) {
	if literal == "" {
		return s, 0
	}
	var b strings.Builder
	total, last := 0, 0
	for _, m := range recipePlaceholder.FindAllStringIndex(s, -1) {
		seg, n := replaceWholeWord(s[last:m[0]], literal, repl)
		b.WriteString(seg)
		b.WriteString(s[m[0]:m[1]])
		total, last = total+n, m[1]
	}
	seg, n := replaceWholeWord(s[last:], literal, repl)
	b.WriteString(seg)
	return b.String(), total + n
}

func replaceWholeWord(s, literal, repl string) (string, int) {
	var b strings.Builder
	n, pos := 0, 0
	for {
		i := strings.Index(s[pos:], literal)
		if i < 0 {
			break
		}
		start, end := pos+i, pos+i+len(literal)
		if isWordBoundary(s, start, end) {
			b.WriteString(s[pos:start])
			b.WriteString(repl)
			n++
		} else {
			b.WriteString(s[pos:end])
		}
		pos = end
	}
	b.WriteString(s[pos:])
	return b.String(), n
}

func isWordBoundary(s string, start, end int) bool {
	before := start == 0 || !isWordByte(s[start-1])
	after := end == len(s) || !isWordByte(s[end])
	return before && after
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// warnLiterals reports value-looking literals left in a step's fields
// after substitution, once per step and literal.
func (c *capture) warnLiterals(steps []RecipeStep) {
	for i := range steps {
		seen := map[string]bool{}
		walkStepStrings(&steps[i], func(p *string) {
			for _, lit := range captureLiteralRe.FindAllString(*p, -1) {
				if seen[lit] {
					continue
				}
				seen[lit] = true
				c.warnf("step %s: %q looks like a value; pass --var <name>=%s to template it", steps[i].ID, lit, lit)
			}
		})
	}
}

// MarshalRecipe renders a recipe as the YAML document ParseRecipe reads:
// header keys in declaration order, empty fields omitted, and none of
// the load-time fields (path, source, hash).
func MarshalRecipe(r *Recipe) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("marshal recipe: nil recipe")
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(r); err != nil {
		return nil, fmt.Errorf("marshal recipe %s: %w", r.Name, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("marshal recipe %s: %w", r.Name, err)
	}
	return buf.Bytes(), nil
}
