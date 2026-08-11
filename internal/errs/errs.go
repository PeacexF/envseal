// Package errs provides envseal's error type: an actionable message,
// a stable exit code, and an optional list of things to check.
//
// Errors must never carry secret material
package errs

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// Code is a process exit code. Scripts depend on these values; keep them stable.
type Code int

const (
	CodeOK       Code = 0
	CodeGeneral  Code = 1
	CodeConfig   Code = 2
	CodeCrypto   Code = 3
	CodeIdentity Code = 4
	CodeProcess  Code = 5
	CodeGit      Code = 6
)

type Error struct {
	Code    Code
	Summary string
	Detail  string
	Checks  []string
	cause   error
	silent  bool
}

// Exit carries a child process's exit code without printing anything: the
// child has already said whatever it had to say.
func Exit(code int) *Error {
	return &Error{Code: Code(code), Summary: fmt.Sprintf("exit status %d", code), silent: true}
}

func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Summary: fmt.Sprintf(format, args...)}
}

func (e *Error) Detailf(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

func (e *Error) Check(items ...string) *Error {
	e.Checks = append(e.Checks, items...)
	return e
}

// Wrap attaches a cause, which is shown to the user.
func (e *Error) Wrap(err error) *Error {
	e.cause = err
	return e
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Summary + ": " + e.cause.Error()
	}
	return e.Summary
}

func (e *Error) Unwrap() error { return e.cause }

// CodeOf reports the exit code for err, defaulting to CodeGeneral.
func CodeOf(err error) Code {
	if err == nil {
		return CodeOK
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return CodeGeneral
}

// Render writes err to w as a human-readable block:
//
//	Error: unable to decrypt .env.enc
//
//	The encrypted file was not encrypted for the current identity.
//
//	Check:
//	  • ~/.envseal/identity
func Render(w io.Writer, err error) {
	if err == nil {
		return
	}

	e, ok := errors.AsType[*Error](err)
	if !ok {
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}
	if e.silent {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Error: %s\n", e.Error())
	if e.Detail != "" {
		fmt.Fprintf(&b, "\n%s\n", e.Detail)
	}
	if len(e.Checks) > 0 {
		b.WriteString("\nCheck:\n")
		for _, c := range e.Checks {
			fmt.Fprintf(&b, "  • %s\n", c)
		}
	}
	io.WriteString(w, b.String())
}
