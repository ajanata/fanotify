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
	"time"

	"github.com/ajanata/faapi"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/rossmcdonald/telegram_hook"
	log "github.com/sirupsen/logrus"
)

type (
	bot struct {
		config *Config
		fa     *faapi.Client
		tg     *tgbotapi.BotAPI
	}
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.Info("FurAffinity notifier bot starting, phase 1")

	c := loadConfig()
	log.WithField("config", c).Debug("Loaded config")

	hook, err := telegram_hook.NewTelegramHook("FANotifierBot", c.TG.Token, c.TG.OwnerID,
		telegram_hook.WithAsync(true), telegram_hook.WithTimeout(15*time.Second))
	if err != nil {
		log.WithError(err).Warn("Unable to create telegram log hook; logs will only be here")
	}
	log.AddHook(hook)
	log.Error("FurAffinity notifier bot starting, phase 2")

	if c.Debug {
		go func() {
			log.Info(http.ListenAndServe("localhost:6680", nil))
		}()
	}

	fa, err := faapi.New(c.FA.faAPIConfig())
	if err != nil {
		log.WithError(err).Error("Unable to create faapi client!")
		panic(err)
	}
	defer fa.Close()

	tg, err := tgbotapi.NewBotAPI(c.TG.Token)
	if err != nil {
		log.WithError(err).Error("Unable to create telegram client!")
		panic(err)
	}
	tg.Debug = c.Debug
	err = tgbotapi.SetLogger(tglogger{})
	if err != nil {
		log.WithError(err).Error("Unable to set telegram client logger")
	}
	log.WithField("username", tg.Self.UserName).Info("Logged in to telegram.")

	bot := bot{
		config: c,
		fa:     fa,
		tg:     tg,
	}
	bot.runLoop()
}

func (b *bot) runLoop() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.tg.GetUpdatesChan(u)
	if err != nil {
		log.WithError(err).Error("Unable to subscribe to updates")
		panic(err)
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.WithFields(log.Fields{
			"from":       update.Message.From.UserName,
			"text":       update.Message.Text,
			"is_command": update.Message.IsCommand(),
		}).Debug("incoming message")
	}
}
