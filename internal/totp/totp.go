package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"strconv"
	"strings"
	"time"
)

// GetToken
// Generate token from input MFA Secret key
func GetToken(secretKey string) string {
	now := time.Now().Unix()
	return strconv.Itoa(int(generateTOTP(secretKey, now)))
}

// generateTOTP function
func generateTOTP(secretKey string, timestamp int64) uint32 {

	// The base32 encoded secret key string is decoded to a byte slice
	base32Decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	secretKey = strings.ToUpper(strings.TrimSpace(secretKey)) // preprocess
	secretBytes, _ := base32Decoder.DecodeString(secretKey)   // decode

	// The truncated timestamp / 30 is converted to an 8-byte big-endian
	// unsigned integer slice
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(timestamp)/30)

	// The timestamp bytes are concatenated with the decoded secret key
	// bytes. Then a 20-byte SHA-1 hash is calculated from the byte slice
	hash := hmac.New(sha1.New, secretBytes)
	hash.Write(timeBytes) // Concat the timestamp byte slice
	h := hash.Sum(nil)    // Calculate 20-byte SHA-1 digest

	// AND the SHA-1 with 0x0F (15) to get a single-digit offset
	offset := h[len(h)-1] & 0x0F

	// Truncate the SHA-1 by the offset and convert it into a 32-bit
	// unsigned int. AND the 32-bit int with 0x7FFFFFFF (2147483647)
	// to get a 31-bit unsigned int.
	truncatedHash := binary.BigEndian.Uint32(h[offset:]) & 0x7FFFFFFF

	// Take modulo 1_000_000 to get a 6-digit code
	return truncatedHash % 1_000_000
}
