// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/flosch/pongo2"
	"github.com/pressly/chi"
	"github.com/sharpner/pobin"
)

var assetfs *pongo2.TemplateSet

func setupTmpl() {
	assetfs = pongo2.NewSet("", pobin.NewMemoryTemplateLoader(rice.MustFindBox("static").Bytes))
}

var assetTimestamp = time.Now().Unix()

func tmpl(w http.ResponseWriter, r *http.Request, path string, ctx map[string]interface{}) {
	atmpl, err := assetfs.FromFile(path)
	if orig, ok := err.(*pongo2.Error); ok {
		if os.IsNotExist(orig.OrigError) {
			NotFoundHandler(w, r)
			return
		}
	}

	tpl := pongo2.Must(atmpl, err)

	if ctx == nil {
		ctx = make(map[string]interface{})
	}

	ctx["full_url"] = r.URL.String()
	ctx["url"] = r.URL
	ctx["cachetag"] = assetTimestamp
	ctx["conf"] = &conf
	ctx["now"] = time.Now()

	cw.RLock()
	ctx["cache"] = cw.cache

	out, err := tpl.ExecuteBytes(ctx)
	cw.RUnlock()
	if err != nil {
		panic(err)
	}

	w.Write(out)
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("url params not allowed in file server")
	}

	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}
