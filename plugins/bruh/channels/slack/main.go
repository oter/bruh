// Command slack is the Slack channel of bruh: a stdio MCP server that brings the answers of
// the owner in Slack threads into the bigm session, and relays its permission prompts.
package main

import (
	"cmp"
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	Token, Channel, DataDir, API string
	Allowed                      []string
	Poll                         time.Duration
	Active                       bool
}

// setting returns an environment value; an unresolved ${user_config.*} reference counts as unset.
func setting(name string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if strings.HasPrefix(v, "${") {
		return ""
	}
	return v
}

func configFromEnv() config {
	c := config{
		Token:   setting("SLACK_BOT_TOKEN"),
		Channel: setting("SLACK_CHANNEL_ID"),
		DataDir: setting("BRUH_DATA"),
		API:     cmp.Or(setting("SLACK_API_URL"), "https://slack.com/api/"),
	}
	for u := range strings.FieldsFuncSeq(setting("SLACK_ALLOWED_USERS"), func(r rune) bool { return r == ',' || r == ' ' }) {
		c.Allowed = append(c.Allowed, u)
	}
	secs, err := strconv.Atoi(setting("SLACK_POLL_SECONDS"))
	if err != nil || secs < 5 {
		secs = 20
	}
	c.Poll = time.Duration(secs) * time.Second
	// Only bigm talks to Slack; every other session that loads the plugin gets an idle server.
	c.Active = os.Getenv("BRUH_ROLE_KEY") == "bigm" && c.Token != "" && c.Channel != "" && len(c.Allowed) > 0 && c.DataDir != ""
	return c
}

func main() {
	cfg := configFromEnv()
	if !strings.HasSuffix(cfg.API, "/") {
		cfg.API += "/"
	}
	s := newServer(cfg, &slackAPI{base: cfg.API, token: cfg.Token, hc: &http.Client{Timeout: 30 * time.Second}}, os.Stdout)
	if err := s.serve(context.Background(), os.Stdin); err != nil {
		log.Fatal(err)
	}
}
