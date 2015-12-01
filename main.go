package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"strconv"
	"time"

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
	UrlBase            string

	RedisHost string
	RedisDb   int
}

var templateManager temple.TemplateStore

func init() {
	rand.Seed(time.Now().UnixNano())
}
func main() {
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/labelmaker/")
	viper.AddConfigPath(".")
	viper.SetDefault("RedisHost", "localhost:6379")
	viper.SetDefault("RedisDb", 1)
	var err error
	if err = viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}
	if err = viper.Unmarshal(&appConfig); err != nil {
		log.Fatal(err)
	}

	pool = newRedisPool(appConfig.RedisHost, appConfig.RedisDb)
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
	r.Use(renderError)
	r.Use(auth.AuthCheck())

	r.GET("/", home)
	r.POST("/hooks/:hook", onHook)

	locked := r.Group("/", auth.RequireAuth())
	locked.GET("/repo/:owner/:name", repo)
	locked.POST("/install/:owner/:name", install)

	r.Run(":9999")
}

func render(c *gin.Context, name string, data interface{}) {
	c.Header("Content-Type", "text/html")
	if err := templateManager.Execute(c.Writer, data, name); err != nil {
		c.AbortWithError(500, err)
	}
}

func randString(l int) string {
	data := make([]byte, l)
	for i := 0; i < l; i++ {
		data[i] = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"[rand.Intn(52)]
	}
	return string(data)
}

func renderError(c *gin.Context) {
	c.Next()
	errs := c.Errors.Errors()
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

func repo(ctx *gin.Context) {
	owner, name := ctx.Param("owner"), ctx.Param("name")
	ri, err := getRepoInfo(owner, name)
	if err != nil {
		ctx.Error(err)
		return
	}
	u := ghauth.User(ctx)
	data := gin.H{"User": u, "Owner": owner, "Name": name, "Info": ri}
	render(ctx, "repo", data)
}

func install(ctx *gin.Context) {
	owner, name := ctx.Param("owner"), ctx.Param("name")
	u := ghauth.User(ctx)

	hookName := "web"
	hook := &github.Hook{}
	hook.Name = &hookName
	hookPath := randString(20)
	hookSecret := randString(20)
	hook.Config = map[string]interface{}{
		"url":          fmt.Sprintf("%s/hooks/%s", appConfig.UrlBase, hookPath),
		"content_type": "json",
		"secret":       hookSecret,
	}
	hook.Events = []string{"issue_comment", "issues", "pull_request_review_comment", "pull_request", "push", "status"}
	hook, _, err := u.Client().Repositories.CreateHook(owner, name, hook)
	if err != nil {
		ctx.Error(err)
		return
	}
	if err = registerHook(owner, name, hookPath, hookSecret, *hook.ID, u.Token); err != nil {
		ctx.Error(err)
		return
	}
	ctx.Redirect(302, fmt.Sprintf("/repo/%s/%s", owner, name))
}

func onHook(ctx *gin.Context) {
	ctx.String(200, "aaa")
	eventType := ctx.Request.Header.Get("X-Github-Event")
	hookId := ctx.Param("hook")
	hi, err := getHookInfo(hookId)
	if err != nil {
		ctx.Error(err)
		return
	}
	//read and mac the body
	body, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.Error(err)
		return
	}
	mac := hmac.New(sha1.New, []byte(hi.Secret))
	_, err = mac.Write(body)
	if err != nil {
		ctx.Error(err)
		return
	}
	//compare to signature header
	signature := ctx.Request.Header.Get("X-Hub-Signature")
	sig, err := hex.DecodeString(signature[5:])
	if err != nil {
		ctx.Error(err)
		return
	}
	fmt.Println(hmac.Equal(mac.Sum(nil), sig), eventType)
}
