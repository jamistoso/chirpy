package auth

import (
    "fmt"
	"testing"
	"time"
	"net/http"

	"github.com/google/uuid"
)

func TestCorrectPass(t *testing.T) {
    pass := "Gladysongno"
	hashPass, err := HashPassword(pass)
    if err != nil {
        t.Fatalf(`HashPassword("%q") = %q, %v`, pass, hashPass, err)
    }
	err = CheckPasswordHash(pass, hashPass)
    if err != nil {
        t.Fatalf(`CheckPasswordHash("Gladys") = %v, wanted nil`, err)
    }
}

func TestIncorrectPass(t *testing.T) {
    pass := "Gladys"
	err := CheckPasswordHash("incorrect", pass)
    if err == nil {
        t.Fatal(`CheckPasswordHash("Gladys") = nil, wanted error`, )
    }
}

func TestCorrectJWT(t *testing.T) {
    userId, _ := uuid.NewUUID()
    delay := time.Duration(1000000000000)
	token, err := MakeJWT(userId, "haha", delay)
    if err != nil {
        t.Fatalf(`MakeJWT(userId, "haha", delay) = %q, %v, wanted token, nil`, token, err)
    }
    returnedId, err := ValidateJWT(token, "haha")
    if (err != nil || returnedId != userId) {
        t.Fatalf(`ValidateJWT(token, "haha") = %q, %v, wanted %q, nil`, returnedId, err, userId)
    }
}

func TestNonmatchingJWT(t *testing.T) {
    userId, _ := uuid.NewUUID()
    delay := time.Duration(1000000000000)
	token, err := MakeJWT(userId, "incorrect", delay)
    if err != nil {
        t.Fatalf(`MakeJWT(userId, "incorrect", delay) = %q, %v, wanted token, nil`, token, err)
    }
    returnedId, err := ValidateJWT(token, "haha")
    expectedError := fmt.Errorf("token has invalid claims: token is expired")
    if (err == nil || userId == returnedId) {
        t.Fatalf(`ValidateJWT(token, "haha") = %q, %v, wanted nil, %v`, returnedId, userId, expectedError)
    }
}

func TestExpiredJWT(t *testing.T) {
    userId, _ := uuid.NewUUID()
    delay := time.Duration(100)
	token, err := MakeJWT(userId, "haha", delay)
    if err != nil {
        t.Fatalf(`MakeJWT(userId, "haha", delay) = %q, %v, wanted token, nil`, token, err)
    }
    returnedId, err := ValidateJWT(token, "haha")
    expectedError := fmt.Errorf("token has invalid claims: token is expired")
    if (err == nil) {
        t.Fatalf(`ValidateJWT(token, "haha") = %q, %v, wanted nil, %v`, returnedId, userId, expectedError)
    }
}

func TestGetBearerPass(t *testing.T) {
    headers := http.Header{}
    tokenString := "Bearer uhuh"
    headers.Add("Authorization", tokenString)
    result, err := GetBearerToken(headers)
    desiredResult := "uhuh"
    if err != nil || result != desiredResult {
        t.Fatalf(`GetBearerToken(headers) = %q, %v, wanted %v, nil`, result, err, tokenString)
    }
}

func TestGetBearerFailIncorrectToken(t *testing.T) {
    headers := http.Header{}
    tokenString := "Bearer nuhuh"
    headers.Add("Authorization", tokenString)
    result, err := GetBearerToken(headers)
    desiredResult := "uhuh"
    if err != nil || result == desiredResult {
        t.Fatalf(`GetBearerToken(headers) = %q, %v, wanted %v, nil`, result, err, tokenString)
    }
}

func TestGetBearerFailNoHeader(t *testing.T) {
    headers := http.Header{}
    result, err := GetBearerToken(headers)
    desiredResult := ""
    if err == nil || result != desiredResult {
        t.Fatalf(`GetBearerToken(headers) = %q, %v, wanted "", nil`, result, err)
    }
}