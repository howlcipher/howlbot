package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Config struct {
	DiscordToken     string
	HowlframeCmd     string
	HowlbotScript    string
	AdminRoleID      string
	MaintainerRoleID string
}

type Event struct {
	Command  string `json:"command"`
	UserID   string `json:"user_id"`
	Roles    string `json:"roles"` // comma separated
	Approved string `json:"approved"`
}

type Decision struct {
	Command      string `json:"command"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	Action       string `json:"action"`
	ActionTarget string `json:"action_target"`
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "howl",
			Description: "HowlBot general commands",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "status",
					Description: "Check status",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "policy",
					Description: "Check policy",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "history",
					Description: "View recent decisions",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
		{
			Name:        "repo",
			Description: "Repository operations",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "status",
					Description: "Check repo status",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "release",
					Description: "Propose a release",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "approve",
					Description: "Approve a pending operation",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "id",
							Description: "Decision ID to approve",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
					},
				},
			},
		},
	}
)

type Bot struct {
	cfg     Config
	Session *discordgo.Session
	history []Decision
}

func NewBot(cfg Config) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		cfg:     cfg,
		Session: dg,
		history: make([]Decision, 0),
	}

	dg.AddHandler(bot.onReady)
	dg.AddHandler(bot.onInteractionCreate)

	return bot, nil
}

func (b *Bot) Start() error {
	err := b.Session.Open()
	if err != nil {
		return err
	}

	log.Println("Registering commands...")
	for _, v := range commands {
		_, err := b.Session.ApplicationCommandCreate(b.Session.State.User.ID, "", v)
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", v.Name, err)
		}
	}

	return nil
}

func (b *Bot) Stop() {
	b.Session.Close()
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	subCmd := ""
	if len(data.Options) > 0 {
		subCmd = data.Options[0].Name
	}
	fullCmd, approved := PolicyCommand(data.Name, subCmd)

	log.Printf("Received interaction: %s", fullCmd)

	// Normalize Identity
	roles := []string{}
	if i.Member != nil {
		for _, roleID := range i.Member.Roles {
			if roleID == b.cfg.AdminRoleID {
				roles = append(roles, "admin")
			}
			if roleID == b.cfg.MaintainerRoleID {
				roles = append(roles, "maintainer")
			}
		}
	}

	userID := ""
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	event := Event{
		Command:  fullCmd,
		UserID:   userID,
		Roles:    strings.Join(roles, ","),
		Approved: approved,
	}

	// Call HowlFrame
	log.Printf("Evaluating policy with HowlFrame...")

	args := []string{
		"-run",
		b.cfg.HowlbotScript,
		"command=" + event.Command,
		"user_id=" + event.UserID,
		"roles=" + event.Roles,
		"approved=" + event.Approved,
	}

	cmd := exec.Command(b.cfg.HowlframeCmd, args...)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()

	if err != nil {
		log.Printf("HowlFrame execution failed: %v\nStderr: %s", err, errb.String())
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Internal Error: Policy evaluation failed.",
			},
		})
		return
	}

	var dec Decision
	err = json.Unmarshal(outb.Bytes(), &dec)
	if err != nil {
		log.Printf("Failed to parse decision: %v\nStdout: %s", err, outb.String())
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Internal Error: Invalid policy decision format.",
			},
		})
		return
	}

	dec.Command = fullCmd
	b.history = append(b.history, dec)
	if len(b.history) > 10 {
		b.history = b.history[len(b.history)-10:]
	}

	log.Printf("Decision: %s (Reason: %s)", dec.Decision, dec.Reason)

	resp := fmt.Sprintf("**Decision:** %s\n**Reason:** %s", dec.Decision, dec.Reason)

	if dec.Decision == "REQUIRE_APPROVAL" {
		resp += "\n\nAn administrator must approve this action. Use `/repo approve <id>`"
	} else if dec.Decision == "ALLOW" {
		if dec.Action == "send_message" {
			resp += fmt.Sprintf("\n\n**Action executed:** %s", dec.ActionTarget)
		} else if dec.Action == "show_history" {
			resp += "\n\n**History:**"
			for idx, h := range b.history {
				resp += fmt.Sprintf("\n%d. %s - %s", idx+1, h.Command, h.Decision)
			}
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: resp,
		},
	})
}

// PolicyCommand maps a slash command to the command name src/howlbot.howl
// matches and the approved flag it is evaluated with. `/howl status` is the
// policy's `status` (not `howl_status`); `/repo <sub>` is `repo_<sub>`.
// `/repo approve` is evaluated as an approved `repo_release` (see issue #1:
// the approval id is not yet checked against a stored request).
func PolicyCommand(name, sub string) (command, approved string) {
	approved = "false"
	switch {
	case sub == "":
		command = name
	case name == "howl":
		command = sub
	default:
		command = name + "_" + sub
	}
	if command == "repo_approve" {
		command, approved = "repo_release", "true"
	}
	return command, approved
}
