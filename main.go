/*
 *
 * Copyright (c) 2018, Andy Janata
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without modification, are permitted
 * provided that the following conditions are met:
 *
 * * Redistributions of source code must retain the above copyright notice, this list of conditions
 *   and the following disclaimer.
 * * Redistributions in binary form must reproduce the above copyright notice, this list of
 *   conditions and the following disclaimer in the documentation and/or other materials provided
 *   with the distribution.
 * * Neither the name of the copyright holder nor the names of its contributors may be used to
 *   endorse or promote products derived from this software without specific prior written
 *   permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR
 * IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND
 * FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
 * DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
 * WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY
 * WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *
 */

package main

import (
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"time"

	"github.com/ajanata/faapi"
	"github.com/ajanata/fanotify/db"
	"github.com/ajanata/telegram_hook"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
)

type (
	bot struct {
		c  *Config
		db db.DB
		fa *faapi.Client
		tg *tgbotapi.BotAPI
	}
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.WithField("error", err).Panic("Caught panic")
		}
	}()

	// Load up the config
	c := loadConfig()

	// Configure logging
	level, err := log.ParseLevel(c.LogLevel)
	if err != nil {
		log.WithField("level", c.LogLevel).Warn("Unable to parse log level, using INFO")
		level = log.InfoLevel
	}
	log.SetLevel(level)

	if c.LogForceColors {
		log.SetFormatter(&log.TextFormatter{ForceColors: true})
	}
	if c.LogJSON {
		log.SetFormatter(&log.JSONFormatter{})
	}

	// Add Telegram log hook
	hook, err := telegram_hook.NewTelegramHook("FANotifierBot", c.TG.Token, strconv.FormatInt(c.TG.OwnerID, 10),
		telegram_hook.WithTimeout(15*time.Second))
	if err != nil {
		log.WithError(err).Fatal("Unable to create telegram log hook.")
	}
	hook.Level = log.InfoLevel
	log.AddHook(hook)

	// And now that we've got logging completely set up, we can start logging what we're doing.
	log.Info("FurAffinity Notifier bot starting")
	log.WithField("config", c).Debug("Loaded config")

	// Reconfigure logging to Telegram to requested log level
	level, err = log.ParseLevel(c.TG.LogLevel)
	if err != nil {
		log.WithField("level", c.TG.LogLevel).Warn("Unable to parse Telegram log level, using WARN")
		level = log.WarnLevel
	}
	hook.Level = level

	// Turn on pprof debugging if requested
	if c.Debug {
		go func() {
			log.Info(http.ListenAndServe("localhost:6680", nil))
		}()
	}

	// Load our database.
	d, err := db.New(c.DB.File)
	if err != nil {
		log.WithError(err).Fatal("Unable to open database.")
	}
	defer d.Close()

	// Create FurAffinity API client.
	fa, err := faapi.New(c.FA.faAPIConfig())
	if err != nil {
		log.WithError(err).Fatal("Unable to create faapi client!")
	}
	defer fa.Close()

	username, err := fa.GetUsername()
	if err != nil {
		log.WithError(err).Error("Not logged in to FurAffinity!")
	} else {
		log.WithField("username", username).Info("Logged in to FurAffinity.")
	}

	// Make the Telegram bot API.
	tg, err := tgbotapi.NewBotAPI(c.TG.Token)
	if err != nil {
		log.WithError(err).Fatal("Unable to create telegram client!")
	}
	tg.Debug = c.TG.Debug
	err = tgbotapi.SetLogger(tglogger{})
	if err != nil {
		log.WithError(err).Fatal("Unable to set telegram client logger")
	}
	log.WithField("username", tg.Self.UserName).Info("Logged in to telegram.")

	// Do some testing on the database.
	oldUser, err := d.GetUser(1234)
	if err != nil {
		log.WithError(err).Fatal("Couldn't load old user")
	}
	if oldUser != nil {
		log.Infof("Old user: %+v", oldUser)
	}

	user := &db.User{
		ID:            1234,
		Username:      "test",
		AlertKeywords: []string{"asdf", "qwer"},
	}
	err = d.SaveUser(user)
	if err != nil {
		log.WithError(err).Fatal("Couldn't save user")
	}

	// Finally, make the bot and run it.
	bot := bot{
		c:  c,
		db: d,
		fa: fa,
		tg: tg,
	}
	bot.runLoop()
}

func (b *bot) runLoop() {
	logger := log.WithField("func", "runLoop")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.tg.GetUpdatesChan(u)
	if err != nil {
		logger.WithError(err).Panic("Unable to subscribe to updates")
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		logger.WithFields(log.Fields{
			"from":       update.Message.From.UserName,
			"text":       update.Message.Text,
			"is_command": update.Message.IsCommand(),
		}).Debug("incoming message")

		if update.Message.IsCommand() {
			b.dispatchCommand(update.Message)
		}
	}
}
