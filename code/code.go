package code

import (
	"math/rand"
	"strings"
	"time"
)

var letters = strings.Split("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", "")

const codeLength = 6

func GenerateRandom() string {
	code := ""
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	for i := 0; i < codeLength; i++ {
		index := r.Intn(len(letters))
		code += letters[index]
	}
	return code
}
