package utils

import (
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func SignToken(userId string, username, role string) (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	jwtExpiresIn := os.Getenv("JWT_EXPIRES_IN")

	claims := jwt.MapClaims{
		"uid":  userId,
		"user": username,
		"role": role,
	}

	if jwtExpiresIn != "" {
		duration, err := time.ParseDuration(jwtExpiresIn)
		if err != nil {
			return "", ErrorHandler(err, "Internal error")
		}
		claims["exp"] = jwt.NewNumericDate(time.Now().Add(duration))
	} else {
		claims["exp"] = jwt.NewNumericDate(time.Now().Add(15 * time.Minute))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", ErrorHandler(err, "Internal error")
	}

	return signedToken, nil
}

var JwtStore = JWTStore{
	Tokens: make(map[string]time.Time),
}

type JWTStore struct {
	mu     sync.Mutex
	Tokens map[string]time.Time
}

func (store *JWTStore) AddToken(token string, expiryTime time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.Tokens[token] = expiryTime
}

func (store *JWTStore) CleanUpExpiredTokens() {
	for {
		time.Sleep(2 * time.Minute)

		store.mu.Lock()
		for token, timeStamp := range store.Tokens {
			if time.Now().After(timeStamp) {
				delete(store.Tokens, token)
			}
		}
		store.mu.Unlock()
	}
}

func (store *JWTStore) IsLoggedOut(token string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.Tokens[token]
	return ok
}
