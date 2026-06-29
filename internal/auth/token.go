// Package auth issues and verifies the JWTs used to authenticate API requests.
// The TokenManager is shared by the login service (which mints tokens) and the
// auth middleware (which verifies them), so both sides agree on claims and
// signing method.
package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload. UserID and Role are the application-specific bits
// the rest of the API authorizes against; the embedded RegisteredClaims carry
// standard fields like expiry and issued-at.
type Claims struct {
	UserID int    `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager mints and verifies signed access tokens.
type TokenManager interface {
	// Generate signs a token for the user and returns it along with its absolute
	// expiry time (handy for reporting expires_in to the client).
	Generate(userID int, role string) (token string, expiresAt time.Time, err error)
	// Parse verifies the signature and expiry and returns the decoded claims.
	Parse(token string) (*Claims, error)
}

type jwtManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager returns a TokenManager that signs HS256 tokens valid for ttl.
func NewJWTManager(secret string, ttl time.Duration) TokenManager {
	return &jwtManager{secret: []byte(secret), ttl: ttl}
}

func (m *jwtManager) Generate(userID int, role string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.ttl)
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.Itoa(userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (m *jwtManager) Parse(tokenStr string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		// Reject tokens signed with anything other than the expected HMAC method
		// to defend against alg-confusion attacks.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return &claims, nil
}
