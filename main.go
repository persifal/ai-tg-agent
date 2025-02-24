package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	tg              *tgbotapi.BotAPI
	anth            *anthropic.Client
	authorizedUsers *AuthorizedUsers
	conf            *Conf
	repo            *InMemoryConvRepo
)

func init() {
	config, err := NewConf()
	if err != nil {
		log.Fatalf("Unable to load conf.yaml, %v", err)
	}

	tg = createTgClient(config)
	anth = createAnthropicClient(config)

	m := make(map[int64]bool)
	for _, v := range config.Bot.Whitelist {
		i, _ := strconv.Atoi(v)
		m[int64(i)] = true
	}

	authorizedUsers = &AuthorizedUsers{users: m}
	if config.Debug {
		log.Printf("authorized users: %v", m)
	}

	repo = NewRepo()
	conf = config
}

func main() {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := tg.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go handle(update.Message)
	}
}

func handle(msg *tgbotapi.Message) {
	setChatTyping(msg.Chat.ID)
	if !authorizedUsers.contains(int64(msg.From.ID)) {
		log.Printf("unauthorized access attempt from user ID: %d", msg.From.ID)
		msg := tgbotapi.NewMessage(msg.Chat.ID, "sorry, you are not authorized to use this bot.")
		send(&msg)
		return
	}

	if msg.IsCommand() {
		handleCommand(msg)
	} else {
		handleMessage(msg)
	}
}

func handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "startdialog":
		_, ok := repo.NewConversation(msg.Chat.ID)
		if ok {
			reply(msg, "conversation started")
		} else {
			reply(msg, "you already in conversation context")
		}
	case "enddialog":
		repo.CloseConversation(msg.Chat.ID)
		reply(msg, "conversation closed and context cleared")
	default:
		reply(msg, fmt.Sprintf("unknown command: %s", msg.Command()))
	}
}

type AuthorizedUsers struct {
	sync.RWMutex
	users map[int64]bool
}

func (a *AuthorizedUsers) contains(id int64) bool {
	a.RLock()
	defer a.RUnlock()
	return a.users[id]
}

func createTgClient(conf *Conf) *tgbotapi.BotAPI {
	tg, err := tgbotapi.NewBotAPI(conf.Bot.Key)
	if err != nil {
		log.Panicf("Unable to connect to bot: %s", err.Error())
	}

	tg.Debug = conf.Debug

	return tg
}

func createAnthropicClient(conf *Conf) *anthropic.Client {
	if conf.Anthropic.Proxy.Enabled {
		proxyUrl, err := url.Parse(conf.Anthropic.Proxy.Url)
		if err != nil {
			log.Fatal("Invalid proxy URL:", err)
			os.Exit(1)
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			},
		}

		return anthropic.NewClient(
			option.WithAPIKey(conf.Anthropic.Key),
			option.WithHTTPClient(httpClient),
		)
	} else {
		return anthropic.NewClient(option.WithAPIKey(conf.Anthropic.Key))
	}
}

func forward(tgMsg *tgbotapi.Message) (*anthropic.Message, error) {
	message, err := anth.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
		MaxTokens: anthropic.F(int64(2048)),
		System:    anthropic.F([]anthropic.TextBlockParam{anthropic.NewTextBlock(conf.Anthropic.System)}),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(tgMsg.Text)),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("forward finished with error: %s", err.Error())
	}

	return message, nil
}

func conversationForward(msg *tgbotapi.Message) (*anthropic.Message, error) {
	conv, ok := repo.Get(msg.Chat.ID)
	if !ok {
		return nil, errors.New("conversation is not started")
	}

	userMessage := anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text))
	repo.AddMessage(msg.Chat.ID, userMessage)

	response, err := anth.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
		MaxTokens: anthropic.F(int64(4096)),
		Messages:  anthropic.F(conv.history),
		System:    anthropic.F([]anthropic.TextBlockParam{anthropic.NewTextBlock(conf.Anthropic.System)}),
	})
	if err != nil {
		return nil, fmt.Errorf("forward finished with error: %s", err.Error())
	}

	repo.AddMessage(msg.Chat.ID, response.ToParam())
	return response, nil
}

func handleMessage(tgMsg *tgbotapi.Message) {
	var anthMsg *anthropic.Message
	var err error
	if repo.Exists(tgMsg.Chat.ID) {
		anthMsg, err = conversationForward(tgMsg)
	} else {
		anthMsg, err = forward(tgMsg)
	}

	if err != nil {
		reply(tgMsg, fmt.Sprintf("error response from anthropic: %s", err.Error()))
		return
	}

	if anthMsg != nil {
		var sb strings.Builder
		for _, c := range anthMsg.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
			} else {
				sb.WriteString(fmt.Sprintf("$%s$", c.Type))
			}
		}

		reply(tgMsg, sb.String())
	} else {
		log.Printf("should not happen but no errors and no anthropic response found.")
		reply(tgMsg, "no errors and no anthropic response found =O")
	}
}

func reply(msg *tgbotapi.Message, content string) {
	slices := slice(content)
	messageID := msg.MessageID
	var reply tgbotapi.MessageConfig
	for _, v := range slices {
		reply = tgbotapi.NewMessage(msg.Chat.ID, v)
		reply.ReplyToMessageID = messageID
		sent := send(&reply)
		messageID = sent.MessageID
	}
}

func slice(content string) []string {
	l := float64(utf8.RuneCountInString(content))
	mx := float64(conf.Bot.MaxContentLen)
	d := l / mx

	if d > 1 {
		parts := int(math.Ceil(d))
		average := int(math.Ceil(l / float64(parts)))
		result := make([]string, 0, parts)

		runes := []rune(content)
		w := 0
		for i := 0; i < parts; i++ {
			end := min(w+average, len(runes))
			result = append(result, string(runes[w:end]))
			w += average
		}

		return result
	}

	return []string{content}
}

func min(a, b int) int {
	if a > b {
		return b
	} else {
		return a
	}
}

func send(msg *tgbotapi.MessageConfig) *tgbotapi.Message {
	msg.ParseMode = "HTML"
	sent, err := tg.Send(msg)
	if err != nil {
		log.Printf("telegram API is unavailable '%v'", err.Error())
	}

	return &sent
}

func setChatTyping(chatId int64) {
	action := tgbotapi.NewChatAction(chatId, tgbotapi.ChatTyping)
	tg.Send(action)
}
