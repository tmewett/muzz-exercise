package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type User struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Gender   string `json:"gender"`
	Age      int    `json:"age"`
}

// GetValidToken attempts to decode the given JWT string and checks it was
// validly signed by a previous call to /login. If successful it returns the
// parsed Token.
func GetValidToken(tokenString string) (*jwt.Token, error) {
	p := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	// TODO Check token expiry.
	return p.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return tokenSecret, nil
	})
}

// POST /user/create
func createUser(c echo.Context) error {
	// Generate a random email address since they must be unique for login.
	address := fmt.Sprintf("address%d@example.com", rand.Uint64())
	// Generate a random location in (-100..100, -100..100).
	x := rand.Float64()*200 - 100
	y := rand.Float64()*200 - 100

	row := dbPool.QueryRow(ctx, "INSERT INTO users (email, name, password, gender, age, location) VALUES ($1, $2, $3, $4, $5, point($6, $7)) RETURNING id",
		address, "Example User", "password123", "male", 30, x, y)
	var userID int
	err := row.Scan(&userID)
	if err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	result := map[string]interface{}{
		"id":       userID,
		"email":    address,
		"name":     "Example User",
		"password": "password123",
		"gender":   "male",
		"age":      30,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"result": result})
}

// POST /login
// Form data email=...&password=...
// Note: I used form data in all handlers so far just for quick convenience to demonstrate,
// but it would be more suitable in a production system to use JSON.
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: strconv.Itoa(userID),
		// TODO include the issued-at field and test it in GetValidToken.
	})
	tokenString, err := token.SignedString(tokenSecret)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"token": tokenString})
}

// GET /discover?user_id=...&genders=g1,g2,...[&min_age=n][&max_age=n]
// Get a list of profiles which haven't been swiped by the given user. Must be authenticated.
// genders is a comma-separated list of strings to be included.
func discover(c echo.Context) error {
	userIDStr := c.QueryParam("user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user_id"})
	}
	// TODO Call GetValidToken with the JWT included in the request and check
	// its subject is the user_id.

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

	var userLocationX, userLocationY float64
	err = dbPool.QueryRow(ctx, "SELECT location[0], location[1] FROM users WHERE id = $1", userID).Scan(&userLocationX, &userLocationY)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get user's location"})
	}

	genders := strings.Split(c.QueryParam("genders"), ",")

	// Here I'm using the PostgreSQL geometry types to do the distance sorting
	// in the actual query, for performance reasons. If there were a large
	// number of users this means we wouldn't have to read every single one into
	// memory and sort them there. (An alternative approach might be to use a
	// regional bucketing system, so we only have to read from nearby
	// "buckets".)
	rows, err := dbPool.Query(ctx, `
		SELECT id, name, age, gender, length(lseg(point($5, $6), location)) AS distance
		FROM users
		WHERE id != $1
		AND age >= $2 AND age <= $3
		AND gender = ANY($4)
		AND id NOT IN (
			SELECT swipee_id
			FROM swipes
			WHERE swiper_id = $1
		)
		ORDER BY distance
	`, userID, minAge, maxAge, genders, userLocationX, userLocationY)
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
		var distance float32
		if err := rows.Scan(&id, &name, &age, &gender, &distance); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to scan user data"})
		}
		user := map[string]interface{}{
			"id":           id,
			"name":         name,
			"age":          age,
			"gender":       gender,
			"distanceToMe": distance,
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error iterating over user data"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"results": users})
}

// POST /swipe
// Form data user_id=...&swipee_id=...&liked=true|false
// Saves that the given user swiped on swipee and whether it was a like or pass.
func swipe(c echo.Context) error {
	swiperIDStr := c.FormValue("user_id")

	// TODO Call GetValidToken with the JWT included in the request and check
	// its subject is the user_id.

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
		// Check the reverse connection to see if there's a match. Note that
		// it's possible for two users to both swipe each other at the same time
		// and both get match notifications, since the inserts and reads may be
		// interleaved like I1,I2,R1,R2. If this is undesirable it can be fixed
		// with a transaction.
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
	return c.JSON(http.StatusOK, map[string]interface{}{"results": results})
}

var (
	ctx         = context.Background()
	dbPool      *pgxpool.Pool
	tokenSecret = []byte("36a4705a0d7759ff71a7e9c0cf788e4040897b689786caccc290e12b2e190dc3")
)

func main() {
	// This, and the signing secret above, should of course be loaded with some
	// kind of configuration management when deployed.
	dsn := "postgresql://postgres:password@localhost:5432/postgres"

	// Save pool to a new variable and then copy into global dbPool, or else
	// we'll shadow it due to the `:=`
	dbPoolNew, err := pgxpool.New(ctx, dsn)
	dbPool = dbPoolNew
	if err != nil {
		log.Fatal(err)
	}
	defer dbPool.Close()

	// Create tables if they don't exist.
	_, err = dbPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			gender VARCHAR(10) NOT NULL,
			age integer NOT NULL,
			location point NOT NULL
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
	e.GET("/discover", discover)
	e.POST("/swipe", swipe)

	e.Logger.Fatal(e.Start("localhost:8080"))
}
