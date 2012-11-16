package main

import (
	"github.com/simonz05/godis/redis"
	"net/url"
	"os"
)

func NewRedisConnection() *redis.Client {
	redisUrl, err := url.Parse(os.Getenv("REDIS_URL"))
	if err != nil {
		panic(err)
	}

	password := ""
	if redisUrl.User != nil {
		password, _ = redisUrl.User.Password()
	}

	return redis.New("tcp:"+redisUrl.Host, 0, password)
}
