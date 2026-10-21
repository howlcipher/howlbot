package adapter

import (
	"errors"
	"github.com/bwmarrin/discordgo"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type responseTransport struct{}

func (responseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
}
func TestUnauthorizedDiscordApproverPreservesRequest(t *testing.T) {
	a := NewApprovals()
	id := a.Request("requester", "repo_release")
	b := &Bot{cfg: Config{AdminRoleID: "admin", HowlframeCmd: "nonexistent-mission007-evaluator"}, approvals: a}
	s, err := discordgo.New("Bot synthetic")
	if err != nil {
		t.Fatal(err)
	}
	s.Client = &http.Client{Transport: responseTransport{}}
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "synthetic", Token: "synthetic", Type: discordgo.InteractionApplicationCommand,
		Member: &discordgo.Member{User: &discordgo.User{ID: "unauthorized"}},
		Data:   discordgo.ApplicationCommandInteractionData{Name: "repo", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "approve", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "id", Type: discordgo.ApplicationCommandOptionString, Value: id}}}}},
	}}
	b.onInteractionCreate(s, i)
	if _, err := a.Approve(id, "authorized-admin"); err != nil {
		t.Fatalf("unauthorized interaction consumed pending request: %v", err)
	}
}

func TestAuthorizedEvaluatorFailurePreservesRequest(t *testing.T) {
	a := NewApprovals()
	id := a.Request("requester", "repo_release")
	b := &Bot{cfg: Config{AdminRoleID: "admin", HowlframeCmd: "nonexistent-mission007-evaluator"}, approvals: a}
	s, err := discordgo.New("Bot synthetic")
	if err != nil {
		t.Fatal(err)
	}
	s.Client = &http.Client{Transport: responseTransport{}}
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "synthetic", Token: "synthetic", Type: discordgo.InteractionApplicationCommand,
		Member: &discordgo.Member{User: &discordgo.User{ID: "admin-user"}, Roles: []string{"admin"}},
		Data:   discordgo.ApplicationCommandInteractionData{Name: "repo", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "approve", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "id", Type: discordgo.ApplicationCommandOptionString, Value: id}}}}},
	}}
	b.onInteractionCreate(s, i)
	if _, err := a.Approve(id, "authorized-admin"); err != nil {
		t.Fatalf("evaluation failure consumed pending request: %v", err)
	}
}

func TestAuthorizedHandlerCommitAndDenialRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("synthetic shell evaluator fixture requires POSIX")
	}
	for _, verdict := range []string{"ALLOW", "DENY"} {
		t.Run(verdict, func(t *testing.T) {
			a := NewApprovals()
			id := a.Request("requester", "repo_release")
			script := filepath.Join(t.TempDir(), "policy")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"decision\":\""+verdict+"\",\"action\":\"send_message\",\"reason\":\"synthetic\"}'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			b := &Bot{cfg: Config{AdminRoleID: "admin", HowlframeCmd: script}, approvals: a}
			s, err := discordgo.New("Bot synthetic")
			if err != nil {
				t.Fatal(err)
			}
			s.Client = &http.Client{Transport: responseTransport{}}
			makeInteraction := func() *discordgo.InteractionCreate {
				return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
					ID: "synthetic", Token: "synthetic", Type: discordgo.InteractionApplicationCommand,
					Member: &discordgo.Member{User: &discordgo.User{ID: "admin-user"}, Roles: []string{"admin"}},
					Data:   discordgo.ApplicationCommandInteractionData{Name: "repo", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "approve", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "id", Type: discordgo.ApplicationCommandOptionString, Value: id}}}}},
				}}
			}
			// Concurrent authenticated interactions exercise the reservation and history
			// paths; an ALLOW may commit only once. DENY must leave the original request.
			var wg sync.WaitGroup
			for n := 0; n < 8; n++ {
				wg.Add(1)
				go func() { defer wg.Done(); b.onInteractionCreate(s, makeInteraction()) }()
			}
			wg.Wait()
			_, err = a.Approve(id, "another-admin")
			if verdict == "ALLOW" && !errors.Is(err, ErrNoSuchRequest) {
				t.Fatalf("successful handler did not consume request: %v", err)
			}
			if verdict == "DENY" && err != nil {
				t.Fatalf("denied handler consumed request: %v", err)
			}
			if verdict == "ALLOW" && len(b.history) != 1 {
				t.Fatalf("ALLOW evaluations committed %d times", len(b.history))
			}
		})
	}
}
