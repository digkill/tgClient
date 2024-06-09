package main

import (
	"context"
	"fmt"
	"github.com/go-faster/errors"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"github.com/gotd/td/examples"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/tg"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Some error occured. Err: %s", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx); err != nil {
		panic(err)
	}
}

func sessionFolder(phone string) string {
	var out []rune
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return "phone-" + string(out)
}

func run(ctx context.Context) error {
	log, _ := zap.NewDevelopment(zap.IncreaseLevel(zapcore.InfoLevel), zap.AddStacktrace(zapcore.FatalLevel))
	defer func() { _ = log.Sync() }()

	appIDString := os.Getenv("APP_ID")
	appID, err := strconv.Atoi(appIDString)
	if err != nil {
		panic(err)
	}
	appHash := os.Getenv("APP_HASH")

	phone := os.Getenv("TG_PHONE")
	if phone == "" {
		return errors.New("no phone")
	}

	// Setting up session storage.
	// This is needed to reuse session and not login every time.
	sessionDir := filepath.Join("session", sessionFolder(phone))
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return err
	}
	logFilePath := filepath.Join(sessionDir, "log.jsonl")

	fmt.Printf("Storing session in %s, logs in %s\n", sessionDir, logFilePath)

	d := tg.NewUpdateDispatcher()
	// ctx, 7487452995, -1002194340569, -6231513767493486637

	//updates.ChannelAccessHasher.SetChannelAccessHash(ctx, 7487452995, -1002194340569, -6231513767493486637)
	//	updates.ChannelAccessHasher.SetChannelAccessHash(ctx, 7487452995, -1002194340569, -6231513767493486637)

	gaps := updates.New(updates.Config{
		Handler: d,
		Logger:  log.Named("gaps"),
		//	AccessHasher:
	})

	// So, we are storing session information in current directory, under subdirectory "session/phone_hash"
	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, "session.json"),
	}

	// Authentication flow handles authentication process, like prompting for code and 2FA password.
	flow := auth.NewFlow(examples.Terminal{PhoneNumber: phone}, auth.SendCodeOptions{})

	// Initializing client from environment.
	// Available environment variables:
	// 	APP_ID:         app_id of Telegram app.
	// 	APP_HASH:       app_hash of Telegram app.
	// 	SESSION_FILE:   path to session file
	// 	SESSION_DIR:    path to session directory, if SESSION_FILE is not set
	client := telegram.NewClient(appID, appHash, telegram.Options{
		Logger:        log,
		UpdateHandler: gaps,
		Middlewares: []telegram.Middleware{
			updhook.UpdateHook(gaps.Handle),
		},
		SessionStorage: sessionStorage, // Setting up session sessionStorage to store auth data.
	})

	// Setup message update handlers.
	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		log.Info("Channel message", zap.Any("message", update.Message))
		return nil
	})

	/*d.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
		log.Info("Message", zap.Any("message", update.Message))
		return nil
	})

	d.OnChannelMessageViews(func(ctx context.Context, e tg.Entities, update *tg.UpdateChannelMessageViews) error {
		log.Info("Views", zap.Any("views", update.Views))
		return nil
	})*/

	return client.Run(ctx, func(ctx context.Context) error {
		// Perform auth if no session is available.
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return errors.Wrap(err, "auth")
		}

		// Fetch user info.
		user, err := client.Self(ctx)
		if err != nil {
			return errors.Wrap(err, "call self")
		}

		return gaps.Run(ctx, client.API(), user.ID, updates.AuthOptions{
			OnStart: func(ctx context.Context) {
				log.Info("Gaps started")

			},
		})
	})
}
