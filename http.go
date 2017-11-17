// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
)

func httpServer() {
	setupTmpl()

	r := chi.NewRouter()

	if cli.Proxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.DefaultCompress)
	r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{Logger: logger}))
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Recoverer)
	FileServer(r, "/static", rice.MustFindBox("static").HTTPBox())
	if cli.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl(w, r, "tmpl/index.html", nil)
	})

	r.Get("/channel/{channel}", channelHandler)
	r.Get("/cwhois/*", cwhoisHandler)
	r.Get("/whois/{user}", whoisHandler)

	r.Get("/{page}", func(w http.ResponseWriter, r *http.Request) {
		tmpl(w, r, fmt.Sprintf("tmpl/%s.html", chi.URLParam(r, "page")), nil)
	})

	r.NotFound(NotFoundHandler)

	srv := &http.Server{
		Addr:         cli.HTTP,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if cli.TLS.Enable {
		logger.Printf("initializing https server on %s", cli.HTTP)
		logger.Fatal(srv.ListenAndServeTLS(cli.TLS.Cert, cli.TLS.Key))
	}

	logger.Printf("initializing http server on %s", cli.HTTP)
	logger.Fatal(srv.ListenAndServe())
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	tmpl(w, r, "tmpl/notfound.html", nil)
}

func cwhoisHandler(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimPrefix(r.URL.Path, "/cwhois/")
	if ch == "" {
		NotFoundHandler(w, r)
		return
	}

	if !strings.HasPrefix(ch, "#") {
		ch = "#" + ch
	}

	out, err := rpcWhois(ch)
	if err != nil {
		if strings.Contains(err.Error(), "not registered") {
			out = err.Error()
		} else {
			panic(err)
		}
	}

	tmpl(w, r, "tmpl/whois.html", map[string]interface{}{"query": ch, "whois": out})
}

func whoisHandler(w http.ResponseWriter, r *http.Request) {
	user := chi.URLParam(r, "user")

	out, err := rpcWhois(user)
	if err != nil {
		if strings.Contains(err.Error(), "not registered") {
			out = err.Error()
		} else {
			panic(err)
		}
	}

	tmpl(w, r, "tmpl/whois.html", map[string]interface{}{"query": user, "whois": out})
}

func channelHandler(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimPrefix(r.URL.Path, "/channel/")
	if ch == "" {
		NotFoundHandler(w, r)
		return
	}

	if !strings.HasPrefix(ch, "#") {
		ch = "#" + ch
	}

	tmpl(w, r, "tmpl/channel.html", map[string]interface{}{"query": ch})
}
