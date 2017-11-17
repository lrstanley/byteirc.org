// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jessevdk/go-flags"
)

var (
	version = "master"
	commit  = "latest"
	date    = "-"
)

type Flags struct {
	Debug      bool   `short:"d" long:"debug" description:"enable debugging (pprof endpoints)"`
	ConfigPath string `short:"c" long:"config-path" description:"path to configuration file" default:"config.toml"`
	HTTP       string `short:"b" long:"http" default:":8080" description:"ip:port pair to bind to"`
	Proxy      bool   `short:"p" long:"behind-proxy" description:"if X-Forwarded-For headers should be trusted"`
	TLS        struct {
		Enable bool   `long:"enable" description:"run tls server rather than standard http"`
		Cert   string `long:"cert" description:"path to ssl cert file"`
		Key    string `long:"key" description:"path to ssl key file"`
	} `group:"TLS Options" namespace:"tls"`
}

type Config struct {
	RPC struct {
		Host          string `toml:"host"`
		Port          int    `toml:"port"`
		Admin         string `toml:"admin"`
		AdminPassword string `toml:"admin_password"`
		User          string `toml:"user"`
		UserPassword  string `toml:"user_password"`
	} `toml:"rpc"`
	Influx struct {
		Endpoint  string `toml:"endpoint"`
		Username  string `toml:"username"`
		Password  string `toml:"password"`
		Database  string `toml:"database"`
		Retention string `toml:"retention"`
	} `toml:"influx"`
	IRCOps []string `toml:"ircops"`
	AUP    []string `toml:"aup"`
}

var cli Flags
var conf Config
var logger *log.Logger

func initLogger() {
	logger = log.New(os.Stdout, "", log.Lshortfile|log.LstdFlags)
	logger.Print("initialized logger")
}

func main() {
	_, err := flags.Parse(&cli)
	if err != nil {
		if FlagErr, ok := err.(*flags.Error); ok && FlagErr.Type == flags.ErrHelp {
			os.Exit(0)
		}

		// go-flags should print to stderr/stdout as necessary, so we won't.
		os.Exit(1)
	}

	_, err = toml.DecodeFile(cli.ConfigPath, &conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	logger = log.New(os.Stdout, "", log.Lshortfile|log.LstdFlags)

	// Run the cache update at least once, just to make sure we can fetch
	// everything before starting the server. Then run it every 30s.
	firstRun := true
	start := make(chan struct{}, 1)
	go func() {
		var errors int
		for {
			err = updateCache()
			if err != nil {
				errors++
				logger.Printf("error updating xmlrpc cache: %s", err)

				if firstRun && errors >= 5 {
					log.Fatalf("too many failures trying to fetch xmlrpc stats: %s", err)
				}

				time.Sleep(15 * time.Second)
				continue
			}

			if firstRun {
				firstRun = false
				close(start)
			}

			time.Sleep(30 * time.Second)
		}
	}()

	<-start
	// Initialize the http/https server.
	httpServer()
}
