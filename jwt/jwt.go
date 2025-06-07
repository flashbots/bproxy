package jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	errInvalidJwt = errors.New("invalid jwt")
)

type Claims struct {
	IssuedAt int64 `json:"iat"`
}

func IssuedAt(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("%w: invalid count of parts (expect 3, got %d): %s",
			errInvalidJwt, len(parts), token,
		)
	}

	bytes, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: failed to base64-decode: %w",
			errInvalidJwt, err,
		)
	}

	claims := &Claims{}
	if err = json.Unmarshal(bytes, claims); err != nil {
		return time.Time{}, fmt.Errorf("%w: failed to json-unmarshal: %w",
			errInvalidJwt, err,
		)
	}

	return time.Unix(claims.IssuedAt, 0), nil
}
