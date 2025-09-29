package utils

import (
	"math/rand"
	"strconv"
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

// NewHashSlotTag returns a redis cluster hash-slot tag like "{abc}".
// It uses a base36-encoded timestamp to be short and sufficiently unique per call.
func NewHashSlotTag() string {
	// base36 timestamp yields short alphanumeric content
	inner := strconv.FormatInt(time.Now().UnixNano(), 36)
	return "{" + inner + "}"
}

// Constants for tagged key composition
const (
	// MinKeySizeForTagged enforces a minimum total key length when tags are used
	MinKeySizeForTagged = 8
	// DefaultTaggedSuffixLen is the fixed suffix length to ensure uniqueness within a batch
	DefaultTaggedSuffixLen = 2
)

// ExtractTagBody returns the substring inside braces if present, else returns the input as-is.
func ExtractTagBody(tag string) string {
	if len(tag) == 0 {
		return tag
	}
	left := -1
	for i := 0; i < len(tag); i++ {
		if tag[i] == '{' {
			left = i
			break
		}
	}
	if left >= 0 {
		for j := left + 1; j < len(tag); j++ {
			if tag[j] == '}' {
				return tag[left+1 : j]
			}
		}
	}
	return tag
}

// ComposeTaggedKey composes a key of exact keySize using the provided tag.
// The format is: "{" + tagInner + "}" + suffix, where tagInner is truncated/padded
// to fit, and suffix is random with at least 1 character.
func ComposeTaggedKey(tag string, keySize int, suffixLen int) string {
	if keySize < MinKeySizeForTagged {
		keySize = MinKeySizeForTagged
	}
	if suffixLen < 1 {
		suffixLen = 1
	}
	innerLen := keySize - 2 - suffixLen
	if innerLen < 1 {
		innerLen = 1
	}
	tagBody := ExtractTagBody(tag)
	tagInner := tagBody
	if len(tagInner) > innerLen {
		tagInner = tagInner[:innerLen]
	} else if len(tagInner) < innerLen {
		// Pad with 'x' to keep deterministic inner segment length
		pad := make([]byte, innerLen-len(tagInner))
		for i := range pad {
			pad[i] = 'x'
		}
		tagInner = tagInner + string(pad)
	}
	suffix := RandomString(suffixLen)
	return "{" + tagInner + "}" + suffix
}

// Base36Padded returns the base36 representation of n, left-padded with '0'
// to width. If the representation is longer than width, the rightmost width
// characters are returned.
func Base36Padded(n int, width int) string {
	if width <= 0 {
		return ""
	}
	s := strconv.FormatInt(int64(n), 36)
	if len(s) > width {
		return s[len(s)-width:]
	}
	if len(s) < width {
		pad := make([]byte, width-len(s))
		for i := range pad {
			pad[i] = '0'
		}
		return string(pad) + s
	}
	return s
}

// ComposeTaggedKeyWithCounter composes a tagged key with a deterministic base36
// counter suffix of a fixed width.
func ComposeTaggedKeyWithCounter(tag string, keySize int, suffixLen int, counter int) string {
	suffix := Base36Padded(counter, suffixLen)
	if keySize < MinKeySizeForTagged {
		keySize = MinKeySizeForTagged
	}
	innerLen := keySize - 2 - suffixLen
	if innerLen < 1 {
		innerLen = 1
	}
	tagBody := ExtractTagBody(tag)
	tagInner := tagBody
	if len(tagInner) > innerLen {
		tagInner = tagInner[:innerLen]
	} else if len(tagInner) < innerLen {
		pad := make([]byte, innerLen-len(tagInner))
		for i := range pad {
			pad[i] = 'x'
		}
		tagInner = tagInner + string(pad)
	}
	return "{" + tagInner + "}" + suffix
}
