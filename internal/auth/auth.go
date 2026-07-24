package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

//自己伺服器的secret，且須從env讀取
const secretKey = "your-secret-key-change-in-production"

func GenerateToken(userId int64) (string, error){
	claims := jwt.MapClaims{
		"user_id": userId,
		"exp": time.Now().Add(time.Hour*24).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secretKey))

	if err != nil {
		return "", fmt.Errorf("auth.GenerateToken: %w", err)
	}

	return  tokenString, nil
}

func ParseToken(tokenString string) (int64, error) {
	parsedToken, err := jwt.Parse(tokenString, func (t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}

		return []byte(secretKey), nil
	})

	if err != nil {
		return 0, fmt.Errorf("auth.ParseToken: %w", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)

	if !ok || !parsedToken.Valid{
		return 0, errors.New("auth.ParseToken: invalid token")
	}

	userIdFloat64, ok := claims["user_id"].(float64)

	if !ok {
        return 0, errors.New("auth.ParseToken: user_id missing or wrong type")
    }

	return int64(userIdFloat64), nil
}