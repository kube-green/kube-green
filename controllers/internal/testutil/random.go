package testutil

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		random, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterBytes))))
		if err != nil {
			panic(fmt.Errorf("random error: %s", err))
		}
		b[i] = letterBytes[random.Int64()]
	}
	return string(b)
}
