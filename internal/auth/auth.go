package auth

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"crypto/rand"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hash_pass, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(hash_pass), nil
}

func CheckPasswordHash(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer: 	"chirpy",
		IssuedAt: 	jwt.NewNumericDate(time.Now()),
		ExpiresAt: 	jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:	userID.String(),
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	
	signedToken, err := jwtToken.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	
	return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}
	keyFunc := func(token *jwt.Token) (interface{}, error){
		// Ensure the signing method is what you expect
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Return the secret key used to sign the token
		return []byte(tokenSecret), nil
	}
	jwtToken, err := jwt.ParseWithClaims(tokenString, claims, keyFunc)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to parse jwt token: %s", err)
	}
	id, err := jwtToken.Claims.GetSubject()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("unable to retrieve user id")
	}
	userId, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("unable to parse user id")
	}
	return userId, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	bearer := headers.Get("Authorization")
	if bearer == "" {
		return "", fmt.Errorf("no bearer token found in headers")
	}
	return strings.TrimPrefix(bearer, "Bearer "), nil
}

func MakeRefreshToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	refToken := hex.EncodeToString(b)
	return refToken, nil
}