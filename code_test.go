package main

import "testing"

func TestGenerateRandomCode(t *testing.T) {
	code := GenerateRandomCode()
	if len(code) != codeLength {
		t.Errorf("wrong length expected: %d got %d", codeLength, len(code))
	}
}
