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
	tooManyMonitorsFormat       = "You're already monitoring %d users, which is the maximum allowed. Please remove a user from monitoring to add a new one."
	tooManyTotalUserMonitorsMsg = "This bot is already monitoring the maximum number of users it is configured to allow."

	addSubmissionsMsg = `Send me a message with the username you wish to monitor for new submissions. It doesn't matter if you don't get the case right.

Or, you can send /cancel to cancel adding a submission alert.`

	delSubmissionsMsgSuffix = `

Please send the username you no longer wish to monitor for new submission.

Or, you can send /cancel to cancel deleting a submission alert.`
)

func (b *bot) cmdAddSubmissions(u *tgbotapi.User) {
	logger := log.WithFields(log.Fields{
		"func":     "cmdAddSubmissions",
		"userID":   u.ID,
		"username": u.UserName,
	})

	if !b.userStartedBot(u.ID) {
		return
	}

	// dbu, err := b.db.GetTGUser(db.TelegramID(u.ID))
	// if err != nil {
	// 	logger.WithError(err).Error("Unable to load user")
	// 	b.sendMessage(u.ID, loadFailedFormat, "your saved user %s alerts")
	// }

	if false {
		logger.Warn("User already at maximum user monitor limit")
		b.sendMessage(u.ID, tooManyMonitorsFormat, b.c.PerUserMaxUserMonitors)
	}

	b.plaintextHandler[u.ID] = b.addSubmissionsCallback
	b.sendMessage(u.ID, addSubmissionsMsg)
}

func (b *bot) addSubmissionsCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "addSubmissionsCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	// TODO make sure it's a valid fa username

	err := b.db.AddUserSubmissionsForUser(db.TelegramID(m.From.ID), m.Text)
	if err != nil {
		logger.WithError(err).Error("Unable to add submissions for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "user submission alert"))
	} else {
		b.sendHTMLMessage(m.From.ID, "I will alert you to any new submissions from <code>%s</code> now.", m.Text)
	}
}

func (b *bot) loadMonitoredUsers(u *tgbotapi.User) (submissionUsers, journalUsers []string, err error) {
	logger := log.WithFields(log.Fields{
		"func":     "loadMonitoredUsers",
		"userID":   u.ID,
		"username": u.UserName,
	})

	user, err := b.db.GetTGUser(db.TelegramID(u.ID))
	if err != nil {
		logger.WithError(err).Error("Could not load user")
		return submissionUsers, journalUsers, err
	}

	for su := range user.SubmissionUsers {
		submissionUsers = append(submissionUsers, su)
	}

	for ju := range user.JournalUsers {
		journalUsers = append(journalUsers, ju)
	}

	return submissionUsers, journalUsers, nil
}

func (b *bot) getMonitoredUsersToSend(u *tgbotapi.User, journals bool) string {
	logger := log.WithFields(log.Fields{
		"func":     "getMonitoredUsersToSend",
		"userID":   u.ID,
		"username": u.UserName,
	})

	if !b.userStartedBot(u.ID) {
		return ""
	}

	su, ju, err := b.loadMonitoredUsers(u)
	which := "submission"
	users := su
	if journals {
		which = "journal"
		users = ju
	}
	if err != nil {
		logger.WithError(err).Error("Could not load user")
		b.sendMessage(u.ID, loadFailedFormat,
			fmt.Sprintf("your saved user %s alerts", which))
		return ""
	}

	if len(users) == 0 {
		b.sendMessage(u.ID, "You don't have any user %s alerts saved. Send /add%s to get started!",
			which, which)
		return ""
	}

	msg := fmt.Sprintf("You have the following user %s alerts saved:", which)
	for _, u := range users {
		msg = fmt.Sprintf("%s\n<code>%s</code>", msg, u)
	}
	return msg
}

func (b *bot) cmdListSubmissions(u *tgbotapi.User) {
	msg := b.getMonitoredUsersToSend(u, false)
	if msg == "" {
		return
	}
	msg = msg + "\n\nSend /delsubmissions to remove one."
	b.sendHTMLMessage(u.ID, msg)
}

func (b *bot) cmdDelSubmissions(u *tgbotapi.User) {
	msg := b.getMonitoredUsersToSend(u, false)
	if msg == "" {
		return
	}

	b.plaintextHandler[u.ID] = b.delSubmissionsCallback
	b.sendHTMLMessage(u.ID, msg+delSubmissionsMsgSuffix)
}

func (b *bot) delSubmissionsCallback(m *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func":     "delSubmissionsCallback",
		"userID":   m.From.ID,
		"username": m.From.UserName,
	})

	err := b.db.DeleteUserSubmissionsForUser(db.TelegramID(m.From.ID), m.Text)
	switch err {
	case db.ErrNoFAUser:
		b.sendMessage(m.From.ID, "I couldn't find that user.")
	case nil:
		b.sendHTMLMessage(m.From.ID, "I will no longer alert you to any new submissions from <code>%s</code>.", m.Text)
	default:
		logger.WithError(err).Error("Unable to delete submissions for user")
		b.sendMessage(m.From.ID, fmt.Sprintf(saveFailedFormat, "user submission alert deletion"))
	}
}
