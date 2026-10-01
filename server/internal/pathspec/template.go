// Package pathspec implements the versioned, language-neutral result path grammar.
package pathspec

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const Default = "{contextId}/{requestId}/{attemptId}/{file}"

var token = regexp.MustCompile(`\{([^{}]+)\}`)
var formats = []struct{ token, layout string }{{"yyyy", "2006"}, {"SSS", "000"}, {"MM", "01"}, {"dd", "02"}, {"HH", "15"}, {"mm", "04"}, {"ss", "05"}}

type Values struct {
	ContextID, RequestID, AttemptID, File string
	SubmittedAt                           time.Time
}

func Name(name string) error {
	if name == "" || !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\{}") {
		return fmt.Errorf("invalid relative path")
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return fmt.Errorf("control character in path")
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, "_linha") {
			return fmt.Errorf("invalid or reserved path segment")
		}
	}
	// Encoded separators/traversal are rejected rather than relying on each transport's decoding order.
	decoded, err := url.PathUnescape(name)
	if err != nil {
		return fmt.Errorf("invalid percent encoding")
	}
	if decoded != name {
		return fmt.Errorf("percent escapes are not accepted in artifact names")
	}
	if len([]byte(name)) > 1024 {
		return fmt.Errorf("path exceeds 1024 bytes")
	}
	return nil
}
func date(format string, t time.Time) (string, error) {
	var out strings.Builder
	for format != "" {
		found := false
		for _, f := range formats {
			if strings.HasPrefix(format, f.token) {
				if f.token == "SSS" {
					fmt.Fprintf(&out, "%03d", t.Nanosecond()/1000000)
				} else {
					out.WriteString(t.Format(f.layout))
				}
				format = format[len(f.token):]
				found = true
				break
			}
		}
		if found {
			continue
		}
		if strings.ContainsRune("/-_TZ", rune(format[0])) {
			out.WriteByte(format[0])
			format = format[1:]
			continue
		}
		return "", fmt.Errorf("unsupported date format")
	}
	return out.String(), nil
}
func Validate(template string) error {
	if len(template) > 1024 || strings.Contains(template, "://") {
		return fmt.Errorf("template must be a relative key")
	}
	for _, id := range []string{"requestId", "attemptId"} {
		n := 0
		for _, part := range strings.Split(template, "/") {
			if part == "{"+id+"}" {
				n++
			}
		}
		if n != 1 || strings.Count(template, "{"+id+"}") != 1 {
			return fmt.Errorf("template requires one complete %s segment", id)
		}
	}
	if strings.Count(template, "{file}") != 1 || !strings.HasSuffix(template, "/{file}") {
		return fmt.Errorf("file must be the final path suffix")
	}
	_, err := expand(template, Values{"context", "request", "attempt", "result.json", time.Date(2026, 9, 28, 23, 59, 58, 123000000, time.UTC)})
	return err
}
func Expand(template string, v Values) (string, error) {
	if err := Validate(template); err != nil {
		return "", err
	}
	for _, id := range []string{v.ContextID, v.RequestID, v.AttemptID} {
		if err := Name(id); err != nil || strings.Contains(id, "/") {
			return "", fmt.Errorf("invalid identity")
		}
	}
	if err := Name(v.File); err != nil {
		return "", err
	}
	return expand(template, v)
}
func expand(template string, v Values) (string, error) {
	var failure error
	result := token.ReplaceAllStringFunc(template, func(s string) string {
		key := s[1 : len(s)-1]
		switch key {
		case "contextId":
			return v.ContextID
		case "requestId":
			return v.RequestID
		case "attemptId":
			return v.AttemptID
		case "file":
			return v.File
		case "submittedAt":
			return v.SubmittedAt.UTC().Format("20060102T150405.000Z")[:15] + fmt.Sprintf("%03dZ", v.SubmittedAt.UTC().Nanosecond()/1000000)
		}
		if strings.HasPrefix(key, "submittedAt:") {
			f := strings.TrimPrefix(key, "submittedAt:")
			if f == "" {
				failure = fmt.Errorf("empty date format")
				return ""
			}
			value, err := date(f, v.SubmittedAt.UTC())
			if err != nil {
				failure = err
			}
			return value
		}
		failure = fmt.Errorf("unknown placeholder %s", key)
		return ""
	})
	if failure != nil {
		return "", failure
	}
	if err := Name(result); err != nil {
		return "", err
	}
	return result, nil
}
