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
	addJournalsMsg = `Send me a message with the username you wish to monitor for new journals. It doesn't matter if you don't get the case right.

Or, you can send /cancel to cancel adding a journal alert.`

	delJournalsMsgSuffix = `

Please send the username you no longer wish to monitor for new journals.

Or, you can send /cancel to cancel deleting a journal alert.`
)

func (b *bot) cmdAddJournals(u *tgbotapi.User) {
	if !b.userStartedBot(u.ID) {
		return
	}

	b.plaintextHandler[u.ID] = b.addJournalsCallback
	b.sendMessage(u.ID, addJournalsMsg)
}

func (b *bot) addJournalsCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "addJournalsCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	// TODO make sure it's a valid fa username

	err := b.db.AddUserJournalsForUser(db.TelegramID(m.From.ID), m.Text)
	if err != nil {
		logger.WithError(err).Error("Unable to add journals for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "user journal alert"))
	} else {
		b.sendHTMLMessage(m.From.ID, "I will alert you to any new journals from <code>%s</code> now.", m.Text)
	}
}

func (b *bot) cmdListJournals(u *tgbotapi.User) {
	msg := b.getMonitoredUsersToSend(u, true)
	if msg == "" {
		return
	}
	msg = msg + "\n\nSend /deljournals to remove one."
	b.sendHTMLMessage(u.ID, msg)
}

func (b *bot) cmdDelJournals(u *tgbotapi.User) {
	msg := b.getMonitoredUsersToSend(u, true)
	if msg == "" {
		return
	}

	b.plaintextHandler[u.ID] = b.delJournalsCallback
	b.sendHTMLMessage(u.ID, msg+delJournalsMsgSuffix)
}

func (b *bot) delJournalsCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "delJournalsCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	err := b.db.DeleteUserJournalsForUser(db.TelegramID(m.From.ID), m.Text)
	switch err {
	case db.ErrNoFAUser:
		b.sendMessage(m.From.ID, "I couldn't find that user.")
	case nil:
		b.sendHTMLMessage(m.From.ID, "I will no longer alert you to any new journals from <code>%s</code>.", m.Text)
	default:
		logger.WithError(err).Error("Unable to delete journals for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "user journal alert deletion"))
	}
}
