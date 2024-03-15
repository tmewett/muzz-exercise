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

func discover(c echo.Context) error {
	userIDStr := c.QueryParam("user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user_id"})
	}

	minAgeStr := c.QueryParam("min_age")
	minAge, err := strconv.Atoi(minAgeStr)
	if err != nil {
		minAge = 0 // Default to 0 if not provided
	}

	maxAgeStr := c.QueryParam("max_age")
	maxAge, err := strconv.Atoi(maxAgeStr)
	if err != nil {
		maxAge = 999 // Default to a high value if not provided
	}

	genders := strings.Split(c.QueryParam("genders"), ",")
	rows, err := dbPool.Query(ctx, `
		SELECT id, name, age, gender
		FROM users
		WHERE id != $1
		AND age >= $2 AND age <= $3
		AND gender = ANY($4)
		AND id NOT IN (
			SELECT swipee_id
			FROM swipes
			WHERE swiper_id = $1
		)
	`, userID, minAge, maxAge, pq.Array(genders))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to query users"})
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id int
		var name string
		var age int
		var gender string
		if err := rows.Scan(&id, &name, &age, &gender); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to scan user data"})
		}
		user := map[string]interface{}{
			"id":     id,
			"name":   name,
			"age":    age,
			"gender": gender,
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error iterating over user data"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"results": users})
}

func swipe(c echo.Context) error {
	swiperIDStr := c.FormValue("user_id")
	swipeeIDStr := c.FormValue("swipee_id")
	likedStr := c.FormValue("liked")

	swiperID, err := strconv.Atoi(swiperIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user_id"})
	}
	swipeeID, err := strconv.Atoi(swipeeIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid swipee_id"})
	}
	liked, err := strconv.ParseBool(likedStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid liked value"})
	}

	_, err = dbPool.Exec(ctx, `
		INSERT INTO swipes (swiper_id, swipee_id, liked)
		VALUES ($1, $2, $3)
		ON CONFLICT DO UPDATE
	`, swiperID, swipeeID, liked)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to swipe"})
	}

	isMatch := false
	if liked {
		err := dbPool.QueryRow(ctx, `
			SELECT liked
			FROM swipes
			WHERE swiper_id = $1 AND swipee_id = $2
		`, swipeeID, swiperID).Scan(&isMatch)
		if err != nil && err != pgx.ErrNoRows {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to check previous like"})
		}
	}

	results := map[string]interface{}{
		"matched": isMatch,
	}
	if isMatch {
		results["matchID"] = swipeeID
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"results": results})}
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
		CREATE TABLE IF NOT EXISTS swipes (
			swiper_id INT NOT NULL,
			swipee_id INT NOT NULL,
			liked BOOLEAN NOT NULL,
			PRIMARY KEY (swiper_id, swipee_id)
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	e.POST("/user/create", createUser)

	e.Logger.Fatal(e.Start("localhost:8080"))
}
