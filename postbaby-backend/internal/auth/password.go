package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMinLength = 10
	argonTime         = 3
	argonMemory       = 64 * 1024
	argonThreads      = 2
	argonKeyLength    = 32
	argonSaltLength   = 16
)

func PasswordMinLength() int {
	return passwordMinLength
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLength)
	saltEncoded := base64.RawStdEncoding.EncodeToString(salt)
	hashEncoded := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		saltEncoded,
		hashEncoded,
	), nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, expectedHash, err := parseArgonHash(encodedHash)
	if err != nil {
		return false, err
	}

	hash := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(expectedHash)))
	if subtle.ConstantTimeCompare(hash, expectedHash) != 1 {
		return false, nil
	}

	return true, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func parseArgonHash(encodedHash string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return argonParams{}, nil, nil, fmt.Errorf("invalid password hash format")
	}
	if parts[1] != "argon2id" {
		return argonParams{}, nil, nil, fmt.Errorf("unsupported password hash algorithm")
	}

	versionRaw := strings.TrimPrefix(parts[2], "v=")
	version, err := strconv.Atoi(versionRaw)
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("parse password hash version: %w", err)
	}
	if version != argon2.Version {
		return argonParams{}, nil, nil, fmt.Errorf("unsupported password hash version")
	}

	var params argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &params.threads); err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("parse password hash params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("decode password salt: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("decode password hash: %w", err)
	}

	return params, salt, hash, nil
}
