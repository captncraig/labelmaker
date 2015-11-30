package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/captncraig/ghauth"
	"github.com/captncraig/temple"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"github.com/spf13/viper"
)

//go:generate templeGen -dir templates -pkg main -var templates -o templates.go

var appConfig struct {
	GithubClientID     string
	GithubClientSecret string
	CookieSecret       string
	DevMode            bool
}

var templateManager temple.TemplateStore

func main() {
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/labelmaker/")
	viper.AddConfigPath(".")
	var err error
	if err = viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}
	if err = viper.Unmarshal(&appConfig); err != nil {
		log.Fatal(err)
	}

	if templateManager, err = temple.New(appConfig.DevMode, templates, "templates"); err != nil {
		log.Fatal(err)
	}
	if !appConfig.DevMode {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	conf := &ghauth.Conf{
		ClientId:     appConfig.GithubClientID,
		ClientSecret: appConfig.GithubClientSecret,
		Scopes:       []string{"user", "read:public_key", "repo"},
		CookieName:   "ghauth",
		CookieSecret: appConfig.CookieSecret,
	}
	auth := ghauth.New(conf)
	auth.RegisterRoutes("/login", "/callback", "/logout", r)
	r.Use(auth.AuthCheck())

	r.GET("/", home)

	//locked := r.Group("/", auth.RequireAuth())
	
	r.Run(":9999")
}

func render(c *gin.Context, name string, data interface{}) {
	c.Header("Content-Type", "text/html")
	if err := templateManager.Execute(c.Writer, data, name); err != nil {
		c.AbortWithError(500, err)
	}
}
func renderError(c *gin.Context) {
	c.Next()
	errs := c.Errors.Errors()
	fmt.Println(errs)
	if len(errs) > 0 {
		u := ghauth.User(c)
		render(c, "error", gin.H{"User": u, "Errors": errs})
	}
}

func getIntQuery(ctx *gin.Context, name string, def int) int {
	q := ctx.Query(name)
	i, err := strconv.Atoi(q)
	if err != nil {
		return def
	}
	return i
}

func home(ctx *gin.Context) {
	u := ghauth.User(ctx)
	data := gin.H{"User": u}
	if u != nil {
		opts := &github.RepositoryListOptions{}
		opts.PerPage = 20
		opts.Page = getIntQuery(ctx, "page", 0)
		opts.Sort = "pushed"
		repos, res, err := u.Client().Repositories.List("", opts)
		if err != nil {
			ctx.Error(err)
			return
		}
		data["Result"] = res
		data["Repos"] = repos
	}
	render(ctx, "home", data)
}
