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
		// submission and journal IDs are probably in different namespaces but their current IDs are far enough apart
		userAlertedForID map[int]map[int64]bool
		userAlertedMutex sync.Mutex
	}

	ptHandler func(message *tgbotapi.Message)
)

const (
	searchResultTemplate = `<b>Search:</b> <code>%s</code>: %s

by %s (%s)
https://www.furaffinity.net/view/%d/`

	submissionTemplate = `<b>Submission:</b> %s

by %s (%s)
https://www.furaffinity.net/view/%d/`

	journalTemplate = `<b>Journal:</b> %s

by %s
https://www.furaffinity.net/journal/%d/`
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
		userAlertedForID: make(map[int]map[int64]bool),
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
				logger.WithField("update", update).Error("Update does not contain a message")
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
	b.processJobs()

	for {
		select {
		case <-b.shouldQuit:
			log.Info("stopping poller")
			return
		case <-b.pollTimer.C:
			b.processJobs()
		}
	}
}

func (b *bot) processJobs() {
	// Everything runs synchronously in this thread right now. This might need changed eventually.
	log.Debug("Starting jobs")
	b.doSearches()
	b.doUserMonitoring()
	log.Debug("Done with jobs")
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

		// we'll need this later but we're goto-ing over it.
		newSubs := make([]*faapi.Submission, 0)

		if len(subs) == 0 {
			goto allOut
		}

		if search.LastID == 0 {
			// first time this search has been run, only store the most recent ID and do nothing else
			goto updateIDOut
		}

		for _, sub := range subs {
			// submissions could be deleted, or just left out from the results randomly
			if sub.ID <= search.LastID {
				break
			}
			newSubs = append(newSubs, sub)
		}

		if len(newSubs) == 0 {
			goto allOut
		}

		if len(newSubs) == len(subs) {
			// be lazy and don't loop on pages yet
			sLogger.Error("Received an entire page of new results, some results missed!")
		}

		// pre-fetch all of the preview images asynchronously
		b.cacheThumbnails(newSubs)

		for i := len(newSubs) - 1; i >= 0; i-- {
			b.alertForSearchResult(newSubs[i], search)
		}

	updateIDOut:
		search.LastID = subs[0].ID
	allOut:
		search.LastRun = time.Now()
		return search.Update()
	})
	if err != nil {
		logger.WithError(err).Error("Unable to process searches")
	}
}

func (b *bot) hasUserSeenID(subID int64, userID int) bool {
	b.userAlertedMutex.Lock()
	defer b.userAlertedMutex.Unlock()

	// TODO evict entries over time
	userAlerts := b.userAlertedForID[userID]
	if userAlerts == nil {
		userAlerts = make(map[int64]bool)
		b.userAlertedForID[userID] = userAlerts
	}
	if userAlerts[subID] {
		return true
	}
	userAlerts[subID] = true
	return false
}

func (b *bot) alertForSearchResult(sub *faapi.Submission, search *db.Search) {
	logger := log.WithFields(log.Fields{
		"func":   "alertForSearchResult",
		"sub":    sub,
		"search": search,
	})

	bb, err := sub.PreviewImage()
	var fb *tgbotapi.FileBytes
	if err != nil {
		logger.WithError(err).Error("Unable to obtain preview image")
	} else {
		fb = &tgbotapi.FileBytes{
			Name:  sub.Title,
			Bytes: bb,
		}
	}
	msg := fmt.Sprintf(searchResultTemplate, escapeHTML(search.Search), escapeHTML(sub.Title), sub.User,
		sub.Rating, sub.ID)
	for uid := range search.Users {
		if b.hasUserSeenID(sub.ID, int(uid)) {
			continue
		}
		b.tryToSendImage(int(uid), fb, msg)
	}
}

func (b *bot) doUserMonitoring() {
	logger := log.WithField("func", "doUserMonitoring")
	logger.Debug("Monitoring users")

	err := b.db.IterateFAUsers(func(faUser *db.FAUser, ul db.UserLoader) error {
		uLogger := logger.WithField("faUser", faUser)
		uLogger.Debug("Iterating user")

		u := b.fa.NewUser(faUser.Username)
		subs, journs, err := u.GetRecent()
		if err != nil {
			return err
		}

		err = b.handleUserSubmissions(faUser, ul, subs)
		if err != nil {
			return err
		}

		err = b.handleUserJournals(faUser, ul, journs)
		if err != nil {
			return err
		}

		faUser.LastRun = time.Now()
		return faUser.Update()
	})

	if err != nil {
		logger.WithError(err).Error("Unable to process users")
	}
}

func (b *bot) handleUserSubmissions(faUser *db.FAUser, ul db.UserLoader, subs []*faapi.Submission) error {
	logger := log.WithFields(log.Fields{
		"func":   "handleUserSubmissions",
		"faUser": faUser,
	})
	newSubs := make([]*faapi.Submission, 0)
	if len(subs) == 0 {
		return nil
	}

	if faUser.LastSubmissionID == 0 {
		// first time this user has been checked, only store the most recent ID and do nothing else
		goto updateIDOut
	}

	for _, sub := range subs {
		// submissions could be deleted, or just left out from the results randomly
		if sub.ID <= faUser.LastSubmissionID {
			break
		}
		newSubs = append(newSubs, sub)
	}

	if len(newSubs) == 0 {
		return nil
	}

	if len(newSubs) == len(subs) {
		// TODO we don't have a way to get multiple pages yet
		logger.Error("Received more submissions than recents can show, some missed!")
	}

	// pre-fetch all of the preview images asynchronously
	b.cacheThumbnails(newSubs)

	for i := len(newSubs) - 1; i >= 0; i-- {
		b.alertForUserSubmission(newSubs[i], faUser)
	}

updateIDOut:
	faUser.LastSubmissionID = subs[0].ID
	return nil
}

func (b *bot) alertForUserSubmission(sub *faapi.Submission, faUser *db.FAUser) {
	logger := log.WithFields(log.Fields{
		"func":   "alertForUserSubmission",
		"sub":    sub,
		"faUser": faUser,
	})

	bb, err := sub.PreviewImage()
	var fb *tgbotapi.FileBytes
	if err != nil {
		logger.WithError(err).Error("Unable to obtain preview image")
	} else {
		fb = &tgbotapi.FileBytes{
			Name:  sub.Title,
			Bytes: bb,
		}
	}

	msg := fmt.Sprintf(submissionTemplate, escapeHTML(sub.Title), sub.User, sub.Rating, sub.ID)
	for uid := range faUser.SubmissionUsers {
		if b.hasUserSeenID(sub.ID, int(uid)) {
			continue
		}
		b.tryToSendImage(int(uid), fb, msg)
	}
}

func (b *bot) handleUserJournals(faUser *db.FAUser, ul db.UserLoader, journs []*faapi.Journal) error {
	logger := log.WithFields(log.Fields{
		"func":   "handleUserJournals",
		"faUser": faUser,
	})
	newJourns := make([]*faapi.Journal, 0)
	if len(journs) == 0 {
		return nil
	}

	if faUser.LastJournalID == 0 {
		// first time this user has been checked, only store the most recent ID and do nothing else
		goto updateIDOut
	}

	for _, journ := range journs {
		// journals could be deleted, or just left out from the results randomly
		if journ.ID <= faUser.LastJournalID {
			break
		}
		newJourns = append(newJourns, journ)
	}

	if len(newJourns) == 0 {
		return nil
	}

	if len(newJourns) == len(journs) {
		// TODO we don't have a way to get multiple pages yet
		logger.Error("Received more journals than recents can show, some missed!")
	}

	for i := len(newJourns) - 1; i >= 0; i-- {
		b.alertForUserJournal(newJourns[i], faUser)
	}

updateIDOut:
	faUser.LastJournalID = journs[0].ID
	return nil
}

func (b *bot) alertForUserJournal(journ *faapi.Journal, faUser *db.FAUser) {
	msg := fmt.Sprintf(journalTemplate, escapeHTML(journ.Title), journ.User, journ.ID)
	for uid := range faUser.JournalUsers {
		if b.hasUserSeenID(journ.ID, int(uid)) {
			continue
		}
		b.sendHTMLMessage(int(uid), msg)
	}
}

func (b *bot) cacheThumbnails(subs []*faapi.Submission) {
	wg := sync.WaitGroup{}
	wg.Add(len(subs))
	for _, sub := range subs {
		go func() {
			// Submission caches the image, so we just need to invoke this to make it download.
			// If there's an error, it won't cache that and will try again later and return the error if it recurs.
			_, _ = sub.PreviewImage()
			wg.Done()
		}()
	}
	wg.Wait()
}
