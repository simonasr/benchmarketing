package utils

import (
	"math/rand"
	"sync"
	"time"
)

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // Number of indices extracted from a 63-bit Int
)

var (
	letters  = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	randPool = sync.Pool{New: func() any {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}}
)

// RandomString generates a random alphanumeric string of the given length.
// It minimizes RNG calls and reduces lock contention using a sync.Pool.
func RandomString(length int) string {
	if length <= 0 {
		return ""
	}

	b := make([]byte, length)
	r := randPool.Get().(*rand.Rand)
	for i, cache, remain := length-1, r.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = r.Int63(), letterIdxMax
		}
		idx := int(cache & letterIdxMask)
		if idx < len(letters) {
			b[i] = letters[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	randPool.Put(r)
	return string(b)
}

// RandomIntn returns, as an int, a non-negative pseudo-random number in [0,n).
// If n <= 0, it returns 0.
func RandomIntn(n int) int {
	if n <= 0 {
		return 0
	}
	r := randPool.Get().(*rand.Rand)
	v := r.Intn(n)
	randPool.Put(r)
	return v
}
