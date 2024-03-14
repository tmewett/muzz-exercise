package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
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
	userID, err := redisClient.Incr(ctx, "user_id").Result()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	user := User{
		Email:    "user@example.com",
		Name:     "John Doe",
		Password: "password123",
		Gender:   "male",
		Age:      30,
	}
	key := fmt.Sprintf("user:%d", userID)
	result := map[string]interface{}{
		"email":    user.Email,
		"name":     user.Name,
		"password": user.Password,
		"gender":   user.Gender,
		"age":      user.Age,
	}
	err = redisClient.HMSet(ctx, key, result).Err()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"result": result})
}

func login(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.Claims{

	})
	return c.JSON(http.StatusOK, map[string]interface{}{"token": })
}

var (
	ctx         = context.Background()
	redisClient *redis.Client
	tokenSecret = "36a4705a0d7759ff71a7e9c0cf788e4040897b689786caccc290e12b2e190dc3"
)

func main() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	e := echo.New()

	e.POST("/user/create", createUser)

	e.Logger.Fatal(e.Start("localhost:8080"))
}
