package main

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port      string
	JwtSecret string
}

func MustLoadConfig() *Config {
	godotenv.Load()
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	jwtSecret = os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		panic("JWT_SECRET is not provided!")
	}
	return &Config{port, jwtSecret}
}
