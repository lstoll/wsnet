package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/ianschenck/envflag"
)

func main() {
	var port, usersFlag string
	var users [][2]string

	flag.StringVar(&port, "port", "", "Port to listen on")
	envflag.StringVar(&port, "PORT", "", "Port to listen on")
	flag.StringVar(&usersFlag, "users", "", "Comma seperated list of user:pass pairs to auth with")
	envflag.StringVar(&usersFlag, "USERS", "", "Comma seperated list of user:pass pairs to auth with")

	envflag.Parse()
	flag.Parse()

	if port == "" {
		log.Fatal("-port/PORT is required")
	}

	if usersFlag != "" {
		for _, up := range strings.Split(usersFlag, ",") {
			u := new([2]string)
			sp := strings.Split(up, ":")
			copy(u[:], sp)
			users = append(users, *u)
		}
	}

	log.Print(port)

	auth := func(fn http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			user, pass, _ := r.BasicAuth()
			if user == "testu" && pass == "password" {
				fn.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", "Basic")
			http.Error(w, "Unauthorized.", 401)
		}
	}
	serveMux := http.NewServeMux()
	serveMux.Handle("/wsnet", auth(sh))
	go func() {
		err = http.Serve(hl, serveMux)
		if err != nil {
			panic(err)
		}
	}()

}
