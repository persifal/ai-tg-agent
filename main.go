package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gopkg.in/yaml.v3"
)

type Conf struct {
	Bot struct {
		Key       string   `yaml:"key"`
		Whitelist []string `yaml:"whitelist"`
	} `yaml:"bot"`
	Anthropic struct {
		Key   string `yaml:"key"`
		Proxy struct {
			Enabled bool   `yaml:"enabled"`
			Url     string `yaml:"url"`
		} `yaml:"proxy"`
	} `yaml:"anthropic"`
}

func LoadConfig(path string) (*Conf, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error resolving config path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	config := &Conf{}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing yaml: %w", err)
	}

	if err := validate(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func validate(config *Conf) error {
	if config.Bot.Key == "" {
		return fmt.Errorf("telegram token is required")
	}
	if len(config.Bot.Whitelist) == 0 {
		return fmt.Errorf("whitelist is empty")
	}
	if config.Anthropic.Key == "" {
		return fmt.Errorf("anthropic API key is required")
	}

	return nil
}

func main() {
	conf, err := LoadConfig("conf.yaml")
	if err != nil {
		log.Fatalf("Unable to load conf.yaml, %v", err)
	}

	ctx := initCtx(conf)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := ctx.tg.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go handleMessage(ctx, update.Message)
	}
}

type Ctx struct {
	tg              *tgbotapi.BotAPI
	ai              *anthropic.Client
	authorizedUsers map[int64]bool
}

func initCtx(conf *Conf) *Ctx {
	m := make(map[int64]bool)
	for _, v := range conf.Bot.Whitelist {
		i, _ := strconv.Atoi(v)
		m[int64(i)] = true
	}

	fmt.Printf("Authorized users: %v", m)

	ctx := &Ctx{
		createTgClient(conf),
		createAnthropicClient(conf),
		m,
	}

	return ctx
}

func createTgClient(conf *Conf) *tgbotapi.BotAPI {
	tg, err := tgbotapi.NewBotAPI(conf.Bot.Key)
	if err != nil {
		log.Panicf("Unable to connect to bot: %s", err.Error())
	}

	tg.Debug = true

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

func forward(client *anthropic.Client, tgMsg *tgbotapi.Message) (*anthropic.Message, error) {
	message, err := client.Messages.New(context.TODO(), anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
		MaxTokens: anthropic.F(int64(1024)),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(tgMsg.Text)),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("forward finished with error: %s", err.Error())
	}

	return message, nil
}

func handleMessage(ctx *Ctx, tgMsg *tgbotapi.Message) {
	if !ctx.authorizedUsers[tgMsg.From.ID] {
		log.Printf("Unauthorized access attempt from user ID: %d", tgMsg.From.ID)
		msg := tgbotapi.NewMessage(tgMsg.Chat.ID, "Sorry, you are not authorized to use this bot.")
		ctx.tg.Send(msg)
		return
	}

	setChatTyping(tgMsg.Chat.ID, ctx.tg)
	aiMsg, err := forward(ctx.ai, tgMsg)
	contents := make([]string, 0)
	if err != nil {
		contents = append(contents, err.Error())
		replyTo(tgMsg.Chat.ID, int64(tgMsg.MessageID), contents, ctx.tg)
		return
	}

	for _, v := range aiMsg.Content {
		text := v.Text
		vLen := len(text)
		if vLen == 0 {
			contents = []string{
				fmt.Sprintf("Empty response from provider with %d len contents", vLen),
			}
			break
		} else if vLen > 4096 {
			contents = append(contents, slice(text)...)
		} else {
			contents = append(contents, text)
		}
	}

	replyTo(tgMsg.Chat.ID, int64(tgMsg.MessageID), contents, ctx.tg)
}

func slice(text string) []string {
	l := len(text)
	r := make([]string, 0)
	p := 0
	for p < l {
		s := text[p:min(p+4096, l)]
		r = append(r, s)
		p += 4096
	}

	return r
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func replyTo(chatId, replyId int64, contents []string, tg *tgbotapi.BotAPI) {
	rep := replyId
	for _, c := range contents {
		msg := tgbotapi.NewMessage(chatId, c)
		msg.ReplyToMessageID = int(rep)
		sent, err := tg.Send(msg)
		if err != nil {
			log.Fatalf("telegram bot is unavailable '%v'", err.Error())
		}

		rep = int64(sent.MessageID)
	}
}

func setChatTyping(chatId int64, bot *tgbotapi.BotAPI) {
	action := tgbotapi.NewChatAction(chatId, tgbotapi.ChatTyping)
	bot.Send(action)
}
