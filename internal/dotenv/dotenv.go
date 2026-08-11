// Package dotenv parses .env files into ordered key/value entries.
//
// Parsing never rewrites the source: a File keeps the original bytes, so
// encrypting a parsed file reproduces it exactly. Errors report line numbers
// and key names, never values, because every value is a potential secret.
//
// Values are taken literally. There is no ${VAR} interpolation: expanding
// variables is the application's job, and doing it here would change what a
// program sees compared to reading the same file itself.
package dotenv

import (
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/PeacexF/envseal/internal/errs"
)

type Entry struct {
	Key   string
	Value string
	Line  int
}

type File struct {
	raw     []byte
	entries []Entry
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errs.New(errs.CodeConfig, "no environment file at %s", path).
				Check("check the path", "run `envseal encrypt <file>` with the correct name")
		}
		return nil, errs.New(errs.CodeConfig, "unable to read %s", path).Wrap(err)
	}
	return Parse(data, path)
}

// Parse reads dotenv content. source names the origin for error messages.
func Parse(data []byte, source string) (*File, error) {
	src := strings.TrimPrefix(string(data), "\ufeff")
	f := &File{raw: data}

	fail := func(line int, format string, args ...any) error {
		return errs.New(errs.CodeConfig, "invalid environment file %s", source).
			Detailf("Line %d: "+format, append([]any{line}, args...)...)
	}

	for pos, line := 0, 1; pos < len(src); {
		lineEnd := strings.IndexByte(src[pos:], '\n')
		if lineEnd < 0 {
			lineEnd = len(src)
		} else {
			lineEnd += pos
		}

		text := strings.TrimSuffix(src[pos:lineEnd], "\r")
		if trimmed := strings.TrimSpace(text); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			pos, line = lineEnd+1, line+1
			continue
		}

		eq := strings.IndexByte(text, '=')
		if eq < 0 {
			return nil, fail(line, "expected KEY=value.")
		}

		key := strings.TrimSpace(text[:eq])
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if err := validateKey(key); err != nil {
			return nil, fail(line, "%s.", err)
		}

		value, next, consumed, err := parseValue(src, pos+eq+1, lineEnd)
		if err != nil {
			return nil, fail(line, "%s.", err)
		}
		if strings.ContainsRune(value, 0) {
			return nil, fail(line, "the value of %s contains a NUL byte", key)
		}

		f.entries = append(f.entries, Entry{Key: key, Value: value, Line: line})
		pos, line = next, line+1+consumed
	}

	return f, nil
}

// parseValue reads the value starting at i, which is just past the '='.
// A quoted value may run past lineEnd; consumed reports the extra lines used.
func parseValue(src string, i, lineEnd int) (value string, next, consumed int, err error) {
	rest := strings.TrimSuffix(src[i:lineEnd], "\r")

	quote := i + len(rest) - len(strings.TrimLeft(rest, " \t"))
	if quote < lineEnd && (src[quote] == '"' || src[quote] == '\'') {
		return parseQuoted(src, quote)
	}

	if c := inlineComment(rest); c >= 0 {
		rest = rest[:c]
	}
	return strings.Trim(rest, " \t"), lineEnd + 1, 0, nil
}

func parseQuoted(src string, open int) (value string, next, consumed int, err error) {
	quote := src[open]

	var b strings.Builder
	for j := open + 1; j < len(src); j++ {
		switch c := src[j]; {
		case c == quote:
			tail, err := trailing(src, j+1)
			if err != nil {
				return "", 0, 0, err
			}
			return b.String(), tail, consumed, nil

		case c == '\\' && quote == '"' && j+1 < len(src):
			j++
			b.WriteString(unescape(src[j]))
			if src[j] == '\n' {
				consumed++
			}

		default:
			if c == '\n' {
				consumed++
			}
			b.WriteByte(c)
		}
	}

	return "", 0, 0, errors.New("unterminated quoted value")
}

// trailing verifies that only whitespace or a comment follows a quoted value,
// and returns the start of the next line.
func trailing(src string, i int) (int, error) {
	lineEnd := strings.IndexByte(src[i:], '\n')
	if lineEnd < 0 {
		lineEnd = len(src)
	} else {
		lineEnd += i
	}

	rest := strings.TrimSpace(strings.TrimSuffix(src[i:lineEnd], "\r"))
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return 0, errors.New("unexpected text after a quoted value")
	}
	return lineEnd + 1, nil
}

func unescape(c byte) string {
	switch c {
	case 'n':
		return "\n"
	case 'r':
		return "\r"
	case 't':
		return "\t"
	case '\\', '"', '\'', '$':
		return string(c)
	default:
		return `\` + string(c)
	}
}

// inlineComment returns the index of a trailing comment, which must be preceded
// by whitespace so that values like KEY=#tag are kept intact.
func inlineComment(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func validateKey(key string) error {
	if key == "" {
		return errors.New("missing a name before '='")
	}
	for _, r := range key {
		switch {
		case r == ' ' || r == '\t':
			return errors.New("the name contains a space")
		case r < 0x20 || r == 0x7f:
			return errors.New("the name contains a control character")
		}
	}
	return nil
}

// Bytes returns the original file content, unmodified.
func (f *File) Bytes() []byte { return f.raw }

// Entries returns the assignments in source order, including duplicate keys.
func (f *File) Entries() []Entry { return f.entries }

func (f *File) Len() int { return len(f.entries) }

// Keys returns the distinct names in source order.
func (f *File) Keys() []string {
	seen := make(map[string]bool, len(f.entries))
	keys := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		if !seen[e.Key] {
			seen[e.Key] = true
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// Map returns the effective values. When a key is repeated, the last one wins,
// matching how a shell would apply the assignments in order.
func (f *File) Map() map[string]string {
	m := make(map[string]string, len(f.entries))
	for _, e := range f.entries {
		m[e.Key] = e.Value
	}
	return m
}

// Get returns the effective value for key, last assignment winning.
func (f *File) Get(key string) (string, bool) {
	for _, e := range slices.Backward(f.entries) {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// Environ returns the effective values as KEY=value pairs for os/exec.
func (f *File) Environ() []string {
	values := f.Map()
	env := make([]string, 0, len(values))
	for _, k := range f.Keys() {
		env = append(env, k+"="+values[k])
	}
	return env
}
