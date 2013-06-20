package main

import (
	"github.com/simonz05/godis/redis"
	"net/url"
	"os"
)

func NewRedisConnection() *redis.Client {
	urlString := os.Getenv("PARTY_CLOUD_REDIS")
	if urlString == "" {
		urlString = "redis://10.0.2.2:6379"
	}
	redisUrl, err := url.Parse(urlString)
	if err != nil {
		panic(err)
	}

	password := ""
	if redisUrl.User != nil {
		password, _ = redisUrl.User.Password()
	}

	return redis.New("tcp:"+redisUrl.Host, 0, password)
}
