package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"strconv"
	"strings"
	"time"
)

func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	digest := pbkdf2.Key([]byte(password), salt, passwordIterations, 32, sha256.New)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", passwordIterations, base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(digest))
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := pbkdf2.Key([]byte(password), salt, iter, len(expected), sha256.New)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func randomToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		sum := hmac.New(sha256.New, []byte(time.Now().String()))
		sum.Write([]byte(fmt.Sprint(time.Now().UnixNano())))
		return base64.RawURLEncoding.EncodeToString(sum.Sum(nil))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
