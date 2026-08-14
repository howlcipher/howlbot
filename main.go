package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/howlcipher/howlbot/adapter"
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Println("DISCORD_TOKEN environment variable required")
		os.Exit(1)
	}
	
	howlframeCmd := os.Getenv("HOWLFRAME_CMD")
	if howlframeCmd == "" {
		howlframeCmd = "howlframe"
	}
	
	howlbotScript := os.Getenv("HOWLBOT_SCRIPT")
	if howlbotScript == "" {
		howlbotScript = "src/howlbot.howl"
	}

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config/howlbot.json"
	}
	
	configBytes, err := os.ReadFile(configFile)
	
	var cfg struct {
		AdminRoleID      string `json:"admin_role_id"`
		MaintainerRoleID string `json:"maintainer_role_id"`
	}
	
	if err == nil {
		json.Unmarshal(configBytes, &cfg)
	}

	botCfg := adapter.Config{
		DiscordToken:     token,
		HowlframeCmd:     howlframeCmd,
		HowlbotScript:    howlbotScript,
		AdminRoleID:      cfg.AdminRoleID,
		MaintainerRoleID: cfg.MaintainerRoleID,
	}

	bot, err := adapter.NewBot(botCfg)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	err = bot.Start()
	if err != nil {
		log.Fatalf("Error starting bot: %v", err)
	}
	defer bot.Stop()

	fmt.Println("HowlBot is running. Press Ctrl+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
