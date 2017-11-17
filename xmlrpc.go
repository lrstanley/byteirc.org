// Copyright (c) Liam Stanley <me@liamstanley.io>. All rights reserved. Use
// of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kolo/xmlrpc"
)

func rpcCall(op bool, args ...interface{}) (string, error) {
	logger.Printf("executing xmlrpc request to %s:%d: %#v", conf.RPC.Host, conf.RPC.Port, args)
	client, err := xmlrpc.NewClient(fmt.Sprintf("http://%s:%d/xmlrpc", conf.RPC.Host, conf.RPC.Port), nil)
	if err != nil {
		return "", err
	}
	defer client.Close()

	var key string

	authUser := conf.RPC.Admin
	authPassword := conf.RPC.AdminPassword
	if !op {
		authUser = conf.RPC.User
		authPassword = conf.RPC.UserPassword
	}

	if err := client.Call("atheme.login", []interface{}{authUser, authPassword}, &key); err != nil {
		return "", err
	}

	callArgs := []interface{}{key, authUser, "*"}
	callArgs = append(callArgs, args...)

	var result interface{}
	if err := client.Call("atheme.command", callArgs, &result); err != nil {
		return "", err
	}

	return result.(string), nil
}

func rpcWhois(target string) (string, error) {
	var result interface{}
	var err error

	if strings.HasPrefix(target, "#") {
		result, err = rpcCall(false, "ChanServ", "INFO", target)
	} else {
		result, err = rpcCall(false, "NickServ", "INFO", target)
	}

	if err != nil {
		return "", err
	}

	if text, ok := result.(string); ok {
		return text, nil
	}

	return "", fmt.Errorf("Unknown response type found: %T: %#v", result, result)
}

func reAllMatchMap(r *regexp.Regexp, input string) map[string]string {
	results := r.FindAllStringSubmatch(input, -1)
	matches := make(map[string]string)

	for i := 0; i < len(results); i++ {
		if len(results[i]) < 3 || results[i][1] == "" {
			continue
		}

		matches[strings.TrimSpace(results[i][1])] = strings.TrimSpace(results[i][2])
	}

	return matches
}
