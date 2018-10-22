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

// userStartedBot checks that the user has started (and hasn't stopped) the bot.
func (b *bot) userStartedBot(userID int) bool {
	logger := log.WithFields(log.Fields{
		"func":   "userStartedBot",
		"userID": userID,
	})

	user, err := b.db.GetUser(db.TelegramID(userID))
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

func (b *bot) send(userID int, m tgbotapi.Chattable) {
	logger := log.WithFields(log.Fields{
		"func":   "sendMessageImpl",
		"userID": userID,
	})

	if !b.userStartedBot(userID) {
		return
	}

	_, err := b.tg.Send(m)
	if err != nil {
		logger.WithError(err).Error("Unable to send message")
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
