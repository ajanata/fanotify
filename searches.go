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

	"github.com/ajanata/fanotify/db"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
)

const (
	addSearchMsg = `Send me a message with the search you wish to perform, exactly how you would enter it in FurAffinity's search box.

Or, you can send /cancel to cancel adding a search alert.`

	delSearchMsgSuffix = `\n\nPlease send the search to delete, exactly as it appears above.

Or, you can send /cancel to cancel deleting a search alert.`
)

func (b *bot) addSearch(u *tgbotapi.User) {
	if !b.userStartedBot(u.ID) {
		return
	}

	b.plaintextHandler[u.ID] = b.addSearchCallback
	b.sendMessage(u.ID, addSearchMsg)
}

func (b *bot) addSearchCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "addSearchCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	err := b.db.AddSearchForUser(db.TelegramID(m.From.ID), m.Text)
	if err != nil {
		logger.WithError(err).Error("Unable to add search for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "search alert"))
	} else {
		b.sendHTMLMessage(m.From.ID, "I will alert you to any new submissions that match <code>%s</code> now.", m.Text)
	}
}

func (b *bot) getSearchesToSend(u *tgbotapi.User) string {
	logger := log.WithFields(log.Fields{
		"func":     "getSearchesToSend",
		"userID":   u.ID,
		"username": u.UserName,
	})

	if !b.userStartedBot(u.ID) {
		return ""
	}

	user, err := b.db.GetUser(db.TelegramID(u.ID))
	if err != nil {
		logger.WithError(err).Error("Could not load user")
		b.sendMessage(u.ID, loadFailedFormat, "your saved searches")
		return ""
	}

	if len(user.Searches) == 0 {
		b.sendMessage(u.ID, "You don't have any searches saved. Send /addsearch to get started!")
		return ""
	}

	msg := "You have the following searches saved:"
	for s := range user.Searches {
		msg = fmt.Sprintf("%s\n<code>%s</code>", msg, s)
	}
	return msg
}

func (b *bot) listSearch(u *tgbotapi.User) {
	msg := b.getSearchesToSend(u)
	if msg == "" {
		return
	}
	msg = msg + "\n\nSend /delsearch to remove one."
	b.sendHTMLMessage(u.ID, msg)
}

func (b *bot) delSearch(u *tgbotapi.User) {
	msg := b.getSearchesToSend(u)
	if msg == "" {
		return
	}

	b.plaintextHandler[u.ID] = b.delSearchCallback
	b.sendHTMLMessage(u.ID, msg+delSearchMsgSuffix)
}

func (b *bot) delSearchCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "delSearchCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	err := b.db.DelSearchForUser(db.TelegramID(m.From.ID), m.Text)
	switch err {
	case db.ErrNoSearch:
		b.sendMessage(m.From.ID, "I couldn't find that search.")
	case nil:
		b.sendHTMLMessage(m.From.ID, "I will no longer alert you to any new submissions that match <code>%s</code>.", m.Text)
	default:
		logger.WithError(err).Error("Unable to delete search for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "search alert deletion"))
	}
}
