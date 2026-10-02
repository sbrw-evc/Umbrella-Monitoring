// Package auth holds local accounts: password hashing, sessions, API tokens,
// built-in roles and the per-request principal with its permissions and
// service scope.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Iterations is the PBKDF2-SHA256 work factor (OWASP 2023 guidance).
// Tests lower it.
var Iterations = 600_000

const saltLen = 16

// HashPassword returns "pbkdf2-sha256$<iter>$<salt>$<hash>".
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// dummyHash keeps the timing of a login for an unknown user close to a real one.
var dummyHash, _ = HashPassword("umbrella-dummy-password")

// CheckPassword compares in constant time. An empty hash never matches.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		hash = dummyHash
		password += "\x00" // never matches the dummy
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ErrWeakPassword explains the password policy.
var ErrWeakPassword = errors.New("пароль: не короче 10 символов, буквы и цифры, не совпадает с логином")

// CheckPolicy enforces the minimal password policy.
func CheckPolicy(password, username string) error {
	if len([]rune(password)) < 10 || len(password) > 256 {
		return ErrWeakPassword
	}
	var letter, digit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if !letter || !digit || strings.EqualFold(password, username) {
		return ErrWeakPassword
	}
	return nil
}

// RandomToken returns a URL-safe random string with the given prefix.
func RandomToken(prefix string, bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

// TokenHash is the stored form of an API token.
func TokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
