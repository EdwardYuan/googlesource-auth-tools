// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Googlesource-cookieauth is a command that writes Netscape cookie file for
// googlesource.com / source.developers.google.com.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/aki237/nscjar"
	"github.com/google/googlesource-auth-tools/credentials"
)

const (
	refreshInterval = 45 * time.Minute
)

var (
	configs StringList

	runAsDaemon = flag.Bool("run-as-daemon", false, "run the process as a daemon. It refreshes the cookies every 45 minutes.")
)

func init() {
	flag.Var(&configs, "c", "configuration parameters to the git command. This can be specified repeatedly.")
}

func main() {
	flag.Parse()
	if *runAsDaemon {
		// See http://man7.org/linux/man-pages/man7/daemon.7.html for
		// the new style daemons.
		timer := time.NewTimer(refreshInterval)
		for {
			if err := writeCookie(context.Background()); err != nil {
				log.Printf("Cannot write cookies: %v", err)
			} else {
				log.Printf("Wrote cookies")
			}
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(refreshInterval)
			<-timer.C
		}
	} else {
		if err := writeCookie(context.Background()); err != nil {
			log.Fatalf("Cannot write cookies: %v", err)
		}
	}
}

func writeCookie(ctx context.Context) error {
	gitBinary, err := credentials.FindGitBinary()
	if err != nil {
		return fmt.Errorf("cannot find the git binary: %v", err)
	}
	gitBinary.Configs = configs
	urls, err := gitBinary.ListURLs(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the list of URLs in git-config: %v", err)
	}
	var hasGoogleSource, hasSourceDevelopers bool
	for _, u := range urls {
		if u.Host == "googlesource.com" && (u.Path == "" || u.Path == "/") {
			hasGoogleSource = true
		}
		if u.Host == "source.developers.google.com" && (u.Path == "" || u.Path == "/") {
			hasSourceDevelopers = true
		}
	}
	if !hasGoogleSource {
		urls = append(urls, &url.URL{Scheme: "https", Host: "googlesource.com"})
	}
	if !hasSourceDevelopers {
		urls = append(urls, &url.URL{Scheme: "https", Host: "source.developers.google.com"})
	}

	cookies := []*http.Cookie{}
	for _, u := range urls {
		token, err := credentials.MakeToken(ctx, gitBinary, u)
		if err != nil {
			return fmt.Errorf("cannot create a token for %s: %v", u, err)
		}
		cookies = append(cookies, credentials.MakeCookies(u, token)...)
	}

	outputFile, err := gitBinary.PathConfig(ctx, "google.cookieFile")
	if err != nil {
		return fmt.Errorf("cannot read google.cookieFile in git-config: %v", err)
	}
	if outputFile == "" {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("cannot get the current user: %v", err)
		}
		outputFile = filepath.Join(u.HomeDir, ".git-credential-cache", "googlesource-cookieauth-cookie")
	}

	var w *os.File
	if outputFile == "-" {
		w = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(outputFile), 0700); err != nil {
			return fmt.Errorf("cannot create the output directory: %v", err)
		}
		w, err = os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("cannot open the output file: %v", err)
		}
		defer w.Close()
	}

	fmt.Fprintf(w, "# Created by %s at %s\n", os.Args[0], time.Now().Format(time.RFC3339))
	p := nscjar.Parser{}
	for _, c := range cookies {
		p.Marshal(w, c)
	}
	return nil
}

type StringList []string

func (l *StringList) Set(s string) error {
	*l = append(*l, s)
	return nil
}

func (l *StringList) String() string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf("%s", *l)
}
