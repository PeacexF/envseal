package dotenv

import (
	"strings"

	"github.com/PeacexF/envseal/internal/errs"
)

// Set returns the file's bytes with key's value replaced, leaving every other
// byte untouched: comments, ordering, blank lines, spacing, and the quoting of
// other variables all survive.
//
// Only the value token is rewritten, so `export KEY = old # note` becomes
// `export KEY = new # note`. When a key is assigned more than once, the last
// assignment is the effective one and the one changed. An unknown key is
// appended at the end.
func (f *File) Set(key, value string) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, errs.New(errs.CodeConfig, "%q is not a usable variable name", key).
			Detailf("%s.", err)
	}

	// A NUL cannot survive: the operating system marks the end of an
	// environment value with one, and the parser rejects it on the way back in.
	// Writing it would produce a sealed file nothing could read again.
	if strings.ContainsRune(value, 0) {
		return nil, errs.New(errs.CodeConfig, "the value for %s contains a NUL byte", key).
			Detailf("Environment variables cannot hold one: it marks the end of a value.")
	}

	target := -1
	for i, e := range f.entries {
		if e.Key == key {
			target = i
		}
	}

	if target < 0 {
		body := f.body
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return []byte(f.prefix + body + key + "=" + render(value, 0) + "\n"), nil
	}

	e := f.entries[target]
	rendered := render(value, e.quote)
	tail := f.body[e.end:]

	// An empty value shares its separating space with a following comment, so
	// `KEY= # note` has nothing between the two. Writing a value straight in
	// would glue it to the '#' and swallow the comment into the value.
	if rendered != "" && strings.HasPrefix(tail, "#") {
		rendered += " "
	}

	return []byte(f.prefix + f.body[:e.start] + rendered + tail), nil
}

// Has reports whether the file assigns key.
func (f *File) Has(key string) bool {
	_, ok := f.Get(key)
	return ok
}

// render writes a value so that parsing it returns exactly what went in. style
// is the quoting the value had before, which is kept when it still works, to
// avoid churning a file with unnecessary changes.
func render(value string, style byte) string {
	switch {
	case value == "":
		// An empty value never needs quotes, whatever the old style was.
		return ""

	case strings.ContainsAny(value, "\"\\\n\r\t"):
		// Only double quotes have escapes, so these force that form.
		return quoteDouble(value)

	case style == '\'':
		if !strings.Contains(value, "'") {
			return "'" + value + "'"
		}
		return quoteDouble(value)

	case style == '"':
		return quoteDouble(value)

	case strings.ContainsAny(value, " '#") || value[0] == '"':
		return quoteDouble(value)
	}
	return value
}

func quoteDouble(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')

	for i := range len(value) {
		switch c := value[i]; c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}

	b.WriteByte('"')
	return b.String()
}
