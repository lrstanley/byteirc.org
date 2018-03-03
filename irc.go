// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluele/gcache"
	influx "github.com/influxdata/influxdb/client/v2"
)

var reAthemeKV = regexp.MustCompile(`(?m)^ *([^:]+?) *: *(.*)$`)
var reNotification = regexp.MustCompile(`(?m)^([0-9]+): \[[^\]]+\] by ([^\s]+) at ([^\s]+) on ([^:]+): (.*)$`)

type Notification struct {
	ID      int    `json:"id"`
	Author  *User  `json:"author"`
	Time    string `json:"time"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

var reChannel = regexp.MustCompile(`(?m)^(\#[^\s]+)\s+([0-9]+)\s+:(.*) \(([^\)]+)\)$`)

type Channel struct {
	Name       string    `json:"name"`
	Count      int       `json:"count"`
	Topic      string    `json:"topic"`
	Author     *User     `json:"author"`
	Founder    *User     `json:"founder"`
	Registered time.Time `json:"registered"`
}

var reUserAccount = regexp.MustCompile(`Information on ([^\s]+) \(account ([^\s]+)\)`)
var reUserMetadata = regexp.MustCompile(`(?m)^ *Metadata *:  *([^=]+?) *= *(.*)$`)

type User struct {
	Nick       string    `json:"nick"`
	Account    string    `json:"account"`
	Registered time.Time `json:"registered"`
	LastAddr   string    `json:"last_addr"`
	LastSeen   time.Time `json:"last_seen"`
	realAddr   string
	email      string
	nicks      string
	Channels   string `json:"channels"`
	LastQuit   string `json:"last_quit"`

	Metadata struct {
		URL         *url.URL `json:"url"`
		DisplayName string   `json:"display_name"`
		Location    string   `json:"location"`
		About       string   `json:"about"`
	} `json:"metadata"`
}

func (u *User) Avatar() string {
	if u != nil && u.email != "" {
		h := md5.New()
		io.WriteString(h, u.email)
		return fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon&s=300", h.Sum(nil))
	}

	return "https://www.gravatar.com/avatar?d=identicon&s=300"
}

var reBot = regexp.MustCompile(`(?m)^ *([0-9]+): ([^ ]+) \(([^@]+)@([^\)]+)\) \[([^\]]+)\]$`)

type Bot struct {
	ID          int    `json:"id"`
	Nick        string `json:"nick"`
	User        string `json:"user"`
	Host        string `json:"host"`
	Description string `json:"description"`
}

type ircCache struct {
	AccountCount int `json:"account_count"`
	NickCount    int `json:"nick_count"`
	ChannelCount int `json:"channel_count"`
	ActiveCount  int `json:"active_count"`

	Notifications []*Notification `json:"notifications"`
	Channels      []*Channel      `json:"channels"`
	Bots          []*Bot          `json:"bots"`
	IRCOps        []*User         `json:"irc_ops"`
}

var gircCache atomic.Value

func updateCache() error {
	logger.Println("updating cache")
	cache := &ircCache{}

	out, err := rpcCall(true, "OperServ", "UPTIME")
	if err != nil {
		return err
	}

	fields := reAllMatchMap(reAthemeKV, out)
	cache.AccountCount, _ = strconv.Atoi(fields["Registered accounts"])
	cache.NickCount, _ = strconv.Atoi(fields["Registered nicknames"])
	cache.ChannelCount, _ = strconv.Atoi(fields["Registered channels"])
	cache.ActiveCount, _ = strconv.Atoi(fields["Users currently online"])

	out, err = rpcCall(false, "InfoServ", "LIST")
	if err != nil {
		return err
	}
	for _, item := range reNotification.FindAllStringSubmatch(out, -1) {
		next := &Notification{Time: item[3], Date: item[4], Message: item[5]}
		next.ID, _ = strconv.Atoi(item[1])
		next.Author, _ = lookupUser(item[2])
		cache.Notifications = append(cache.Notifications, next)
	}

	out, err = rpcCall(false, "ALIS", "LIST", "*", "-show", "t", "-min", "3", "-topic", "?")
	if err != nil {
		return err
	}
	var chInfo string
	for _, item := range reChannel.FindAllStringSubmatch(out, -1) {
		next := &Channel{Name: item[1], Topic: item[3]}
		next.Count, _ = strconv.Atoi(item[2])
		next.Author, _ = lookupUser(item[4])

		chInfo, err = rpcCall(false, "ChanServ", "INFO", item[1])
		if err != nil {
			cache.Channels = append(cache.Channels, next)
			continue
		}

		fields := reAllMatchMap(reAthemeKV, chInfo)
		next.Founder, _ = lookupUser(fields["Founder"])
		next.Registered = ircTime(fields["Registered"])
		cache.Channels = append(cache.Channels, next)
	}

	sort.Slice(cache.Channels, func(i, j int) bool {
		return cache.Channels[i].Count > cache.Channels[j].Count
	})

	out, err = rpcCall(false, "BotServ", "BOTLIST")
	if err != nil {
		return err
	}
	for _, item := range reBot.FindAllStringSubmatch(out, -1) {
		bot := &Bot{Nick: item[2], User: item[3], Host: item[4], Description: item[5]}
		bot.ID, _ = strconv.Atoi(item[1])
		cache.Bots = append(cache.Bots, bot)
	}

	for _, nick := range conf.IRCOps {
		user, err := lookupUser(nick)
		if err != nil {
			logger.Printf("error looking up ircop %s: %s", nick, err)
			continue
		}

		cache.IRCOps = append(cache.IRCOps, user)
	}

	gircCache.Store(cache)

	// Push metrics as well.
	if conf.Influx.Endpoint == "" {
		return nil
	}

	metrics, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr:     conf.Influx.Endpoint,
		Username: conf.Influx.Username,
		Password: conf.Influx.Password,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		return err
	}
	defer metrics.Close()

	batch, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Database:        conf.Influx.Database,
		RetentionPolicy: conf.Influx.Retention,
	})
	if err != nil {
		return err
	}

	point, err := influx.NewPoint("stats", nil, map[string]interface{}{
		"accounts": cache.AccountCount,
		"nicks":    cache.NickCount,
		"channels": cache.ChannelCount,
		"active":   cache.ActiveCount,
	}, time.Now())
	if err != nil {
		return err
	}
	batch.AddPoint(point)

	logger.Printf("writing metrics to %q:%q", conf.Influx.Endpoint, conf.Influx.Database)
	return metrics.Write(batch)
}

var ulCache = gcache.New(50).LRU().Build()

func lookupUser(nick string) (*User, error) {
	nick = strings.ToLower(nick)

	userCache, err := ulCache.Get(nick)
	if err != nil {
		if err != gcache.KeyNotFoundError {
			logger.Printf("unable to pull nick %s from cache: %s", nick, err)
		}
	} else {
		return userCache.(*User), nil
	}

	result, err := rpcCall(true, "NickServ", "INFO", nick)
	if err != nil {
		return &User{Nick: nick}, err
	}

	logger.Printf("nick %s is a cache miss", nick)

	fields := reAllMatchMap(reAthemeKV, result)

	user := &User{
		Registered: ircTime(fields["Registered"]),
		LastAddr:   fields["Last addr"],
		LastSeen:   ircTime(fields["Last seen"]),
		realAddr:   fields["Real addr"],
		email:      strings.Replace(fields["Email"], " (hidden)", "", -1),
		nicks:      fields["Nicks"],
		Channels:   fields["Channels"],
		LastQuit:   fields["Last quit"],
	}

	ircTime(fields["Registered"])

	accountInfo := reUserAccount.FindStringSubmatch(result)
	if len(accountInfo) == 3 {
		user.Nick = accountInfo[1]
		user.Account = accountInfo[2]
	}

	if user.Nick == "" {
		user.Nick = nick
	}

	// Check if there is any useful metadata.
	metadata := reAllMatchMap(reUserMetadata, result)
	user.Metadata.URL, _ = url.Parse(metadata["URL"])
	user.Metadata.Location = metadata["LOCATION"]
	user.Metadata.DisplayName = metadata["DISPLAY"]
	user.Metadata.About = metadata["ABOUT"]

	if err = ulCache.SetWithExpire(nick, user, 15*time.Minute); err != nil {
		logger.Printf("unable to set cache for nick %s: %s", nick, err)
	}

	return user, nil
}

func ircTime(input string) time.Time {
	if strings.ToLower(input) == "now" {
		return time.Now()
	}

	// Jun 10 22:19:08 2015 -0400 (2y 21w 4d ago)
	if i := strings.Index(input, "("); i > -1 {
		input = strings.TrimSpace(input[:i])
	}

	// Jun 10 22:19:08 2015 -0400
	out, err := time.Parse("Jan 02 15:04:05 2006 -0700", input)
	if err != nil {
		return time.Time{}
	}

	return out
}
