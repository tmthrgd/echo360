package main

import (
	"strings"

	"github.com/gosuri/uiprogress/util/strutil"
)

// strutilResize resizes the string with the given length. It ellipses
// with '...' when the string's length exceeds the desired length or pads
// spaces to the left of the string when length is smaller than desired.
//
// It is a fork of strutil.Resize that pads to the left.
func strutilResize(s string, length uint) string {
	n := int(length)
	if len(s) == n {
		return s
	}
	// Pads only when length of the string smaller than len needed.
	s = strutil.PadLeft(s, n, ' ')
	if len(s) > n {
		var buf strings.Builder
		for i := 0; i < n-3; i++ {
			buf.WriteByte(s[i])
		}
		buf.WriteString("...")
		s = buf.String()
	}
	return s
}
