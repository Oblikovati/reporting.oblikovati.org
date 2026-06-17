// SPDX-License-Identifier: Apache-2.0

package auth

import "testing"

func TestVerifyAcceptsCorrectToken(t *testing.T) {
	body := []byte(`{"comment":"hi"}`)
	if !Verify(body, Token(body)) {
		t.Error("Verify rejected the correct token")
	}
	// Tolerates surrounding whitespace (some clients pad header values).
	if !Verify(body, "  "+Token(body)+"\t") {
		t.Error("Verify should trim whitespace")
	}
}

func TestVerifyRejectsWrongOrEmptyToken(t *testing.T) {
	body := []byte(`{"comment":"hi"}`)
	if Verify(body, "deadbeef") {
		t.Error("Verify accepted a wrong token")
	}
	if Verify(body, "") {
		t.Error("Verify accepted an empty token")
	}
	// A token correct for different bytes must not pass.
	if Verify(body, Token([]byte("other"))) {
		t.Error("Verify accepted a token for different bytes")
	}
}
