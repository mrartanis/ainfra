// Package version implements Debian Policy's epoch/upstream/revision ordering.
package version

import (
	"errors"
	"regexp"
	"strings"
)

var valid = regexp.MustCompile(`^(?:[0-9]+:)?[0-9][A-Za-z0-9.+:~\-]*$`)

func split(s string) (epoch, up, rev string, err error) {
	if len(s) > 500 || !valid.MatchString(s) {
		return "", "", "", errors.New("invalid Debian version")
	}
	epoch = "0"
	if i := strings.IndexByte(s, ':'); i >= 0 {
		epoch = s[:i]
		s = s[i+1:]
		if len(s) == 0 || s[0] < '0' || s[0] > '9' {
			return "", "", "", errors.New("invalid version")
		}
	}
	up = s
	rev = "0"
	if i := strings.LastIndexByte(s, '-'); i >= 0 {
		up = s[:i]
		rev = s[i+1:]
		if rev == "" {
			return "", "", "", errors.New("empty revision")
		}
	}
	return
}
func num(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func digit(c byte) bool { return c >= '0' && c <= '9' }
func order(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	c := s[i]
	if c == '~' {
		return -1
	}
	if digit(c) {
		return 0
	}
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
		return int(c)
	}
	return int(c) + 256
}
func part(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for (i < len(a) && !digit(a[i])) || (j < len(b) && !digit(b[j])) {
			x, y := order(a, i), order(b, j)
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			if i < len(a) {
				i++
			}
			if j < len(b) {
				j++
			}
		}
		si, sj := i, j
		for i < len(a) && digit(a[i]) {
			i++
		}
		for j < len(b) && digit(b[j]) {
			j++
		}
		if c := num(a[si:i], b[sj:j]); c != 0 {
			return c
		}
	}
	return 0
}
func Compare(a, b string) (int, error) {
	ae, au, ar, e := split(a)
	if e != nil {
		return 0, e
	}
	be, bu, br, e := split(b)
	if e != nil {
		return 0, e
	}
	if c := num(ae, be); c != 0 {
		return c, nil
	}
	if c := part(au, bu); c != 0 {
		return c, nil
	}
	return part(ar, br), nil
}
