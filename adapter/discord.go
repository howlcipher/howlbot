package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

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
	cfg       Config
	Session   *discordgo.Session
	historyMu sync.Mutex
	history   []Decision
	approvals *Approvals
}

func NewBot(cfg Config) (*Bot, error) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		cfg:       cfg,
		Session:   dg,
		history:   make([]Decision, 0),
		approvals: NewApprovals(),
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
	approveID := ""
	if len(data.Options) > 0 {
		subCmd = data.Options[0].Name
		for _, opt := range data.Options[0].Options {
			if opt.Name == "id" {
				approveID = opt.StringValue()
			}
		}
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
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	// /repo approve reserves only after authenticated role authorization.
	var reservation *approvalReservation
	if data.Name == "repo" && subCmd == "approve" {
		authorized := false
		for _, role := range roles {
			if role == "admin" {
				authorized = true
			}
		}
		r, err := b.approvals.Reserve(approveID, userID, authorized)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "**Decision:** DENY\n**Reason:** " + err.Error()},
			})
			return
		}
		reservation = r
		defer reservation.cancel() // Retain pending requests on evaluator errors or DENY.
		fullCmd, approved = r.command, "true"
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

	if reservation != nil && dec.Decision == "ALLOW" {
		if err := reservation.commit(); err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "**Decision:** DENY\n**Reason:** " + err.Error()},
			})
			return
		}
	}
	dec.Command = fullCmd
	b.historyMu.Lock()
	b.history = append(b.history, dec)
	if len(b.history) > 10 {
		b.history = b.history[len(b.history)-10:]
	}

	history := append([]Decision(nil), b.history...)
	b.historyMu.Unlock()

	log.Printf("Decision: %s (Reason: %s)", dec.Decision, dec.Reason)

	resp := fmt.Sprintf("**Decision:** %s\n**Reason:** %s", dec.Decision, dec.Reason)

	if dec.Decision == "REQUIRE_APPROVAL" {
		id := b.approvals.Request(userID, fullCmd)
		resp += fmt.Sprintf("\n\nAnother administrator must approve this request: `/repo approve %s`", id)
	} else if dec.Decision == "ALLOW" {
		if dec.Action == "send_message" {
			resp += fmt.Sprintf("\n\n**Action executed:** %s", dec.ActionTarget)
		} else if dec.Action == "show_history" {
			resp += "\n\n**History:**"
			for idx, h := range history {
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
// matches. `/howl status` is the policy's `status` (not `howl_status`);
// `/repo <sub>` is `repo_<sub>`. It never marks anything approved: `/repo
// approve <id>` is resolved against a stored pending request (Approvals).
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
	return command, approved
}
