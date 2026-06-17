// SPDX-License-Identifier: Apache-2.0

// Package auth implements the open endpoint's lightweight request check: the Authorization
// header must equal the CRC-32 (IEEE) of the exact request body, lowercase zero-padded hex.
// This is a cheap probe filter — it stops dumb scanners, not a determined forger — matching
// the token the application computes in its report.Token.
package auth

import (
	"crypto/subtle"
	"fmt"
	"hash/crc32"
	"strings"
)

// Token is the expected Authorization value for body: the lowercase hex CRC-32 (IEEE).
func Token(body []byte) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE(body))
}

// Verify reports whether header carries the correct CRC token for body. The comparison is
// constant-time and tolerates surrounding whitespace in the header.
func Verify(body []byte, header string) bool {
	want := Token(body)
	got := strings.TrimSpace(header)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
