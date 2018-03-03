// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/lrstanley/pt"
)

var tmpl *pt.Loader

func httpServer() {
	tmpl = pt.New("", pt.Config{
		CacheParsed: !cli.Debug,
		Loader:      rice.MustFindBox("static").Bytes,
		ErrorLogger: os.Stderr,
		DefaultCtx: func(w http.ResponseWriter, r *http.Request) (ctx map[string]interface{}) {
			ctx = pt.M{
				"conf":  &conf,
				"now":   time.Now(),
				"cache": gircCache.Load().(*ircCache),
			}
			return ctx
		},
		NotFoundHandler: http.NotFound,
	})

	r := chi.NewRouter()

	if cli.Proxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.DefaultCompress)
	r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{Logger: logger}))
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Recoverer)

	pt.FileServer(r, "/static", rice.MustFindBox("static").HTTPBox())

	if cli.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Render(w, r, "/tmpl/index.html", nil)
	})

	r.Get("/channel/{channel}", channelHandler)
	r.Get("/cwhois/*", cwhoisHandler)
	r.Get("/whois/{user}", whoisHandler)

	r.Get("/{page}", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Render(w, r, fmt.Sprintf("/tmpl/%s.html", chi.URLParam(r, "page")), nil)
	})

	r.Get("/api/cache", func(w http.ResponseWriter, r *http.Request) {
		pt.JSON(w, r, gircCache.Load().(*ircCache))
	})

	r.NotFound(notFoundHandler)

	srv := &http.Server{
		Addr:         cli.HTTP,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if cli.Debug {
		srv.WriteTimeout = 40 * time.Second
	}

	if cli.TLS.Enable {
		logger.Printf("initializing https server on %s", cli.HTTP)
		logger.Fatal(srv.ListenAndServeTLS(cli.TLS.Cert, cli.TLS.Key))
	}

	logger.Printf("initializing http server on %s", cli.HTTP)
	logger.Fatal(srv.ListenAndServe())
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	tmpl.Render(w, r, "/tmpl/notfound.html", nil)
}

func cwhoisHandler(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimPrefix(r.URL.Path, "/cwhois/")
	if ch == "" {
		notFoundHandler(w, r)
		return
	}

	if !strings.HasPrefix(ch, "#") {
		ch = "#" + ch
	}

	out, err := rpcWhois(ch)
	if err != nil {
		if strings.Contains(err.Error(), "not registered") || strings.Contains(err.Error(), "missing params") {
			out = "Unknown channel or not registered."
		} else {
			panic(err)
		}
	}

	tmpl.Render(w, r, "/tmpl/whois.html", pt.M{"query": ch, "whois": out})
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

	tmpl.Render(w, r, "/tmpl/whois.html", pt.M{"query": user, "whois": out})
}

func channelHandler(w http.ResponseWriter, r *http.Request) {
	ch := strings.TrimPrefix(r.URL.Path, "/channel/")
	if ch == "" {
		notFoundHandler(w, r)
		return
	}

	if !strings.HasPrefix(ch, "#") {
		ch = "#" + ch
	}

	tmpl.Render(w, r, "/tmpl/channel.html", pt.M{"query": ch})
}
