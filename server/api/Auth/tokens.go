package Auth

import "github.com/golang-jwt/jwt/v5"

var jwtKey = []byte("your-secret-key") // ← use env var in real app!

func IsValid(tokenStr string) bool {
	_, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return err == nil
}