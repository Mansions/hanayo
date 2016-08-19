package main

import (
	"database/sql"
	"encoding/gob"
	"fmt"

	"git.zxq.co/ripple/hanayo/apiclient"
	"git.zxq.co/ripple/schiavolib"
	"git.zxq.co/x/rs"
	"github.com/gin-gonic/contrib/gzip"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/thehowl/conf"
)

var (
	config struct {
		DSN string

		CookieSecret string

		RedisEnable         bool
		RedisMaxConnections int
		RedisNetwork        string
		RedisAddress        string
		RedisPassword       string

		AvatarURL string
		BaseURL   string

		API       string
		APISecret string
	}
	db *sql.DB
)

func main() {
	fmt.Println("hanayo v0.0.1")

	err := conf.Load(&config, "hanayo.conf")
	switch err {
	case nil:
		// carry on
	case conf.ErrNoFile:
		conf.Export(config, "hanayo.conf")
		fmt.Println("The configuration file was not found. We created one for you.")
		return
	default:
		panic(err)
	}

	var configDefaults = map[*string]string{
		&config.CookieSecret: rs.String(46),
		&config.AvatarURL:    "https://a.ripple.moe",
		&config.BaseURL:      "https://ripple.moe",
		&config.API:          "http://localhost:40001/api/v1/",
		&config.APISecret:    "Potato",
	}
	for key, value := range configDefaults {
		if *key == "" {
			*key = value
		}
	}

	db, err = sql.Open("mysql", config.DSN)
	if err != nil {
		panic(err)
	}

	if gin.Mode() == gin.DebugMode {
		fmt.Println("Development environment detected. Starting fsnotify on template folder...")
		err := reloader()
		if err != nil {
			fmt.Println(err)
		}
	}

	schiavo.Prefix = "hanayo"
	schiavo.Bunker.Send(fmt.Sprintf("STARTUATO, mode: %s", gin.Mode()))

	fmt.Println("Starting session system...")
	var store sessions.Store
	if config.RedisMaxConnections != 0 {
		store, err = sessions.NewRedisStore(
			config.RedisMaxConnections,
			config.RedisNetwork,
			config.RedisAddress,
			config.RedisPassword,
			[]byte(config.CookieSecret),
		)
		if err != nil {
			fmt.Println(err)
			store = sessions.NewCookieStore([]byte(config.CookieSecret))
		}
	} else {
		store = sessions.NewCookieStore([]byte(config.CookieSecret))
	}
	gobRegisters := []interface{}{
		[]message{},
		errorMessage{},
		infoMessage{},
		neutralMessage{},
		warningMessage{},
		successMessage{},
	}
	for _, el := range gobRegisters {
		gob.Register(el)
	}

	fmt.Println("Importing templates...")
	loadTemplates("")

	fmt.Println("Setting up rate limiter...")
	setUpLimiter()

	apiclient.Key = config.APISecret
	apiclient.APIBase = config.API

	fmt.Println("Starting webserver...")

	r := gin.Default()

	r.Use(
		gzip.Gzip(gzip.DefaultCompression),
		sessions.Sessions("session", store),
		sessionInitializer(),
		rateLimiter(false),
	)

	r.Static("/static", "static")

	r.POST("/login", loginSubmit)
	r.GET("/logout", logout)
	r.GET("/u/:user", userProfile)

	loadSimplePages(r)

	r.NoRoute(notFound)

	conf.Export(config, "hanayo.conf")

	r.Run(":45221")
}
