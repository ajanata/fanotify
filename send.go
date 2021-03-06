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
	"strings"

	"github.com/ajanata/fanotify/db"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
)

// blockedUsers indicates whether we were told that the user blocked us.
// TODO it'd be grand to just stop the bot for those users so we stop trying to send to them, but passing that
// particular error out to where we have a writable db transaction requires more refactoring than I want to do right
// now, so perhaps eventually we'll have a CLI to manually update the database once in a while.
// And *even better* for that would be being able to completely delete them and any searches they were the only user
// we were doing this for.
var blockedUsers = map[int]bool{}

// userStartedBot checks that the user has started (and hasn't stopped) the bot.
func (b *bot) userStartedBot(userID int) bool {
	logger := log.WithFields(log.Fields{
		"func":   "userStartedBot",
		"userID": userID,
	})

	user, err := b.db.GetTGUser(db.TelegramID(userID))
	if err != nil {
		logger.WithError(err).Error("Unable to load user")
		return false
	}

	if user == nil {
		return false
	}

	return user.Started
}

// sendMessage checks that the user has started (and hasn't stopped) the bot before sending a message to them.
func (b *bot) sendMessage(userID int, msg string, params ...interface{}) {
	m := tgbotapi.NewMessage(int64(userID), fmt.Sprintf(msg, params...))
	b.send(userID, m)
}

func (b *bot) sendHTMLMessage(userID int, msg string, params ...interface{}) {
	m := tgbotapi.NewMessage(int64(userID), fmt.Sprintf(msg, params...))
	m.ParseMode = "HTML"
	b.send(userID, m)
}

func escapeHTML(s string, params ...interface{}) string {
	html := fmt.Sprintf(s, params...)
	html = strings.Replace(html, "&", "&amp;", -1)
	html = strings.Replace(html, "<", "&lt;", -1)
	html = strings.Replace(html, ">", "&gt;", -1)
	return html
}

func (b *bot) send(userID int, m tgbotapi.Chattable) {
	if blockedUsers[userID] {
		return
	}
	logger := log.WithFields(log.Fields{
		"func":    "send",
		"userID":  userID,
		"message": m,
	})

	if !b.userStartedBot(userID) {
		return
	}

	_, err := b.tg.Send(m)
	if err != nil {
		// TODO better way to check this
		if strings.Contains(err.Error(), "bot was blocked") {
			blockedUsers[userID] = true
			logger.Info("bot was blocked by user, not sending them anything else")
		} else {
			logger.WithError(err).Error("Unable to send message")
		}
	}
}

// tryToSendImage will send an image message if fb is non-nil, with msg as its HTML caption.
// Otherwise, it will just send msg as a regular HTML message.
func (b *bot) tryToSendImage(userID int, fb *tgbotapi.FileBytes, msg string) {
	if fb != nil {
		m := tgbotapi.NewPhotoUpload(int64(userID), *fb)
		// media uploads have a 200 character limit
		if len(msg) > 200 {
			msg = msg[:200]
		}
		m.Caption = msg
		m.ParseMode = "HTML"
		b.send(userID, m)
	} else {
		b.sendHTMLMessage(userID, msg)
	}
}

// alwaysSendMessage always sends a message to the user, even if they haven't started the bot.
// This should only be used when we fail to save that they have started the bot.
func (b *bot) alwaysSendMessage(userID int, msg string) {
	logger := log.WithFields(log.Fields{
		"func":   "alwaysSendMessage",
		"userID": userID,
	})

	m := tgbotapi.NewMessage(int64(userID), msg)
	_, err := b.tg.Send(m)
	if err != nil {
		logger.WithError(err).Error("Unable to send message")
	}
}
