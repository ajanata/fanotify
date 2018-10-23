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
	"sync"
	"time"

	"github.com/ajanata/faapi"
	"github.com/ajanata/fanotify/db"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
)

type (
	bot struct {
		c                *Config
		db               db.DB
		fa               *faapi.Client
		tg               *tgbotapi.BotAPI
		plaintextHandler map[int]ptHandler
		shouldQuit       chan struct{}
		backgroundJobs   sync.WaitGroup
		pollTimer        *time.Ticker
	}

	ptHandler func(message *tgbotapi.Message)
)

func newBot(c *Config, d db.DB, fa *faapi.Client, tg *tgbotapi.BotAPI) *bot {
	// this is so dumb
	pi, err := time.ParseDuration(c.FA.PollInterval.String())
	if err != nil {
		panic(err)
	}

	return &bot{
		c:                c,
		db:               d,
		fa:               fa,
		tg:               tg,
		plaintextHandler: make(map[int]ptHandler),
		shouldQuit:       make(chan struct{}),
		pollTimer:        time.NewTicker(pi),
	}
}

func (b *bot) run() {
	logger := log.WithField("func", "run")

	defer b.pollTimer.Stop()
	b.backgroundJobs.Add(1)
	go b.poller()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.tg.GetUpdatesChan(u)
	if err != nil {
		logger.WithError(err).Panic("Unable to subscribe to updates")
	}

	for {
		select {
		case <-b.shouldQuit:
			return
		case update := <-updates:
			if update.Message == nil {
				break
			}

			logger.WithFields(log.Fields{
				"from":       update.Message.From.UserName,
				"text":       update.Message.Text,
				"is_command": update.Message.IsCommand(),
			}).Debug("incoming message")

			if update.Message.IsCommand() {
				b.dispatchCommand(update.Message)
			} else if handler, exists := b.plaintextHandler[update.Message.From.ID]; exists {
				handler(update.Message)
				delete(b.plaintextHandler, update.Message.From.ID)
			}
		}
	}
}

func (b *bot) poller() {
	defer logPanic()
	defer b.backgroundJobs.Done()

	for {
		select {
		case <-b.shouldQuit:
			log.Info("stopping poller")
			return
		case <-b.pollTimer.C:
			b.doSearches()
		}
	}
}

func (b *bot) doSearches() {
	logger := log.WithField("func", "doSearches")
	logger.Debug("Running searches")

	b.db.IterateSearches(func(search *db.Search, ul db.UserLoader) error {
		logger.WithField("search", search).Debug("Iterating search")
		return nil
	})
}
