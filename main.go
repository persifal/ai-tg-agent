package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	anthropic_api_key = os.Getenv("ANTHROPIC_API_KEY")
	token             = os.Getenv("TELEGRAM_AI_BOT_TOKEN")
	authorizedUsers   map[int64]bool
)

func main() {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account: %s", bot.Self.UserName)
	if bot.Debug {
		keys := make([]int64, 0)
		for k := range authorizedUsers {
			keys = append(keys, k)
		}
		log.Printf("Allowed users are: %v", keys)
	}

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go handleMessage(bot, update.Message)
	}
}

func init() {
	log.Println("Executed init")
	authorizedUsers = make(map[int64]bool)
	if allowedUsers := os.Getenv("AI_AGENT_ALLOWED_USERS"); allowedUsers != "" {
		for _, userIDRaw := range strings.Split(allowedUsers, ",") {
			userID := strings.Trim(userIDRaw, " ")
			if id, err := strconv.ParseInt(userID, 10, 64); err == nil {
				authorizedUsers[id] = true
			}
		}
	}
}

func forwardToAnthropic(tgMessage *tgbotapi.Message) string {
	client := anthropic.NewClient(
		option.WithAPIKey(anthropic_api_key),
	)

	message, err := client.Messages.New(context.TODO(), anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
		MaxTokens: anthropic.F(int64(2048)),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(tgMessage.Text)),
		}),
	})
	if err != nil {
		return fmt.Sprintf("Request to AI provider finished with error: %s", err.Error())
	}

	return message.Content[0].Text
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if !authorizedUsers[message.From.ID] {
		log.Printf("Unauthorized access attempt from user ID: %d", message.From.ID)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Sorry, you are not authorized to use this bot.")
		bot.Send(msg)
		return
	}

	setChatTyping(message.Chat.ID, bot)
	anthropicResponseContent := forwardToAnthropic(message)
	if len(anthropicResponseContent) == 0 {
		anthropicResponseContent = "Empty response from provider"
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, anthropicResponseContent)
	msg.ReplyToMessageID = message.MessageID
	bot.Send(msg)
}

func setChatTyping(chatId int64, bot *tgbotapi.BotAPI) {
	action := tgbotapi.NewChatAction(chatId, tgbotapi.ChatTyping)
	bot.Send(action)
}
