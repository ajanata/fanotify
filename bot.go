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
	"fmt"
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

var (
	emptySearches = make(map[string]bool)
)

const (
	searchResultTemplate = `New submission for search <code>%s</code> by %s:

<a href="https://www.furaffinity.net/view/%s">%s</a> (%s)`
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
	// fire it once at the start
	b.doSearches()

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

	err := b.db.IterateSearches(func(search *db.Search, ul db.UserLoader) error {
		sLogger := logger.WithField("search", search)
		sLogger.Debug("Iterating search")

		s := b.fa.NewSearch(search.Search)
		subs, err := s.GetPage(1)
		if err != nil {
			return err
		}

		if len(subs) == 0 {
			// only warn on this once
			if !emptySearches[search.Search] {
				emptySearches[search.Search] = true
				sLogger.Warn("No results")
			}
			return nil
		}

		newSubs := make([]*faapi.Submission, 0)
		if search.LastID == "" {
			// first time this search has been run, only store the most recent ID and do nothing else
			goto out
		}

		for _, sub := range subs {
			if sub.ID == search.LastID {
				break
			}
			newSubs = append(newSubs, sub)
		}

		if len(newSubs) == 0 {
			return nil
		}

		if len(newSubs) == len(subs) {
			// be lazy and don't loop on pages yet
			sLogger.Error("Received an entire page of new results, some results missed!")
		}

		for _, sub := range newSubs {
			bb, err := sub.PreviewImage()
			var fb *tgbotapi.FileBytes
			if err != nil {
				sLogger.WithField("submission", sub).WithError(err).Error("Unable to obtain preview image")
			} else {
				fb = &tgbotapi.FileBytes{
					Name:  sub.Title,
					Bytes: bb,
				}
			}
			msg := fmt.Sprintf(searchResultTemplate, search.Search, sub.User, sub.ID, sub.Title, sub.Rating)
			for uid := range search.Users {
				if fb != nil {
					m := tgbotapi.NewPhotoUpload(int64(uid), *fb)
					m.Caption = msg
					m.ParseMode = "HTML"
					b.send(int(uid), m)
				} else {
					b.sendHTMLMessage(int(uid), msg)
				}
			}
		}

	out:
		search.LastID = subs[0].ID
		search.LastRun = time.Now()
		return search.Update()
	})
	if err != nil {
		logger.WithError(err).Error("Unable to process searches")
	}
}
