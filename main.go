package main

//go:generate templeGen -dir templates -o templates.go

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/captncraig/ssgo"
	"github.com/captncraig/temple"
	"github.com/google/go-github/github"
)

var (
	flagDev     = flag.Bool("d", false, "Dev mode")
	templeStore temple.TemplateStore
	gh          ssgo.SSO
)

func init() {
	flag.Parse()
	var err error
	templeStore, err = temple.New(*flagDev, templates, "templates")
	if err != nil {
		log.Fatal(err)
	}

	var clientId, clientSecret string
	if clientId = os.Getenv("GH_CLIENT_ID"); clientId == "" {
		log.Fatal("GH_CLIENT_ID required")
	}
	if clientSecret = os.Getenv("GH_CLIENT_SECRET"); clientSecret == "" {
		log.Fatal("GH_CLIENT_SECRET required")
	}
	gh = ssgo.NewGithub(clientId, clientSecret, "write:repo_hook", "public_repo")

}

func main() {
	http.HandleFunc("/login", gh.RedirectToLogin)
	http.HandleFunc("/ghauth", gh.ExchangeCodeForToken)
	h("/", home)
	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		gh.ClearCookie(w)
		http.Redirect(w, r, "/", 302)
	})

	log.Fatal(http.ListenAndServe(":8787", nil))
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	// a 401 from github often means our token has been revoked. Lets clear our cookie and start over.
	if strings.Contains(err.Error(), "401 Bad credentials") {
		gh.ClearCookie(w)
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(500)
	templeStore.Execute(w, err.Error(), "error")
}

var loggedOutContext = &BaseContext{UserId: 0, UserName: "", ImageURL: ""}

// handler type for my entire app.
type labelMakerHandler func(w http.ResponseWriter, r *http.Request, ctx *BaseContext, client *github.Client) error

// my "middleware" handler. Looks up token, gets github user, executes inner handler.
// handles errors appropriately.
func h(route string, f labelMakerHandler) {
	http.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		c := gh.LookupToken(r)
		if c == nil {
			if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/html")
				templeStore.Execute(w, loggedOutContext, "loggedOut")
				return
			} else if route == "/" {
				http.NotFound(w, r)
				return
			} else {
				http.Redirect(w, r, "/", 302)
				return
			}
		}

		client := github.NewClient(c.Client)
		u, _, err := client.Users.Get("")
		if err != nil {
			handleError(w, r, err)
			return
		}
		ctx := &BaseContext{UserId: *u.ID, UserName: *u.Login, ImageURL: *u.AvatarURL}
		err = f(w, r, ctx, client)
		if err != nil {
			handleError(w, r, err)
		}
	})
}

func home(w http.ResponseWriter, r *http.Request, ctx *BaseContext, client *github.Client) error {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return nil
	}
	listOpts := &github.RepositoryListOptions{}
	listOpts.Direction = "desc"
	listOpts.Sort = "pushed"
	listOpts.PerPage = 100
	allRepos, _, err := client.Repositories.List("", listOpts)
	if err != nil {
		return err
	}
	homeCtx := HomeContext{ctx, []*Repository{}}
	for _, repo := range allRepos {
		homeCtx.Repos = append(homeCtx.Repos, &Repository{*repo.ID, *repo.Owner.Login, *repo.Name})
	}

	w.Header().Set("Content-Type", "text/html")
	return templeStore.Execute(w, homeCtx, "loggedIn")
}
