package main

import (
	"math/rand"
	"strings"
	"time"
)

var codeLetters = strings.Split("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", "")

const codeLength = 6

func GenerateRandomCode() string {
	code := ""
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	for i := 0; i < codeLength; i++ {
		index := r.Intn(len(codeLetters))
		code += codeLetters[index]
	}
	return code
}
