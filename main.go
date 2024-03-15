package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"

	"github.com/labstack/echo/v4"
	"net/http"
)

type User struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Gender   string `json:"gender"`
	Age      int    `json:"age"`
}

func GetValidToken(tokenString string) (*jwt.Token, error) {
	p := jwt.NewParser(jwt.WithValidMethods("HS256"))
	return p.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return tokenSecret, nil
	})
}

func createUser(c echo.Context) error {
	address := fmt.Sprintf("address%d@example.com", rand.Uint64())
	row := dbPool.QueryRow(ctx, "INSERT INTO users (email, name, password, gender, age) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		address, "Example User", "password123", "male", 30)

	var userID int
	err := row.Scan(&userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	result := map[string]interface{}{
		"id": userID,
		"email":    address,
		"name":     "Example User",
		"password": "password123",
		"gender":   "male",
		"age":      30,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"result": result})
}

func login(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	var userID int
	err := dbPool.QueryRow(ctx, "SELECT id FROM users WHERE email = $1 AND password = $2", email, password).Scan(&userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to authenticate user"})
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.Claims{
		Subject: strconv.Itoa(userID),
	})
	tokenString, err := token.SignedString(tokenSecret)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"token": tokenString})
}
}

var (
	ctx         = context.Background()
	dbPool *pgxpool.Pool
	tokenSecret = []byte("36a4705a0d7759ff71a7e9c0cf788e4040897b689786caccc290e12b2e190dc3")
)

func main() {
	dsn := "postgresql://username:password@localhost:5432/dbname"
	dbPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer dbPool.Close()

	// Create users table if it doesn't exist
	_, err = dbPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id integer PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			gender VARCHAR(10) NOT NULL,
			age integer NOT NULL
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	e.POST("/user/create", createUser)

	e.Logger.Fatal(e.Start("localhost:8080"))
}
