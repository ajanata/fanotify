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
	"github.com/ajanata/fanotify/db"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	log "github.com/sirupsen/logrus"
)

const (
	saveFailedFormat = "Sorry, I was unable to save that %s. The botmaster has been notified. Please try again later."
	loadFailedFormat = "Sorry, I was unable to load %s. The botmaster has been notified. Please try again later."

	startedMsg = `Welcome to the FurAffinity Notifier bot.

Please consult the /help for a list of commands.`

	helpMsg = `FurAffinity Notifier bot will perform searches or monitor user submissions and journals and alert you when there are new items.
In most circumstances, you should only be notified once about a particular submission. If the bot is restarted, you <i>may</i> get a duplicate notification, if it matches more than one trigger.

This bot is still in development. Not all features are complete, and it may not have great uptime.

<b>Searches are not yet actually performed.</b>
/addsearch: Add a search.
/delsearch: Delete a search.
/listsearch: List saved searches.`
)

func (b *bot) dispatchCommand(cmd *tgbotapi.Message) {
	logger := log.WithFields(log.Fields{
		"func": "dispatchCommand",
		"from": cmd.From.UserName,
		"cmd":  cmd.Text,
	})
	logger.Debug("Received command")

	switch cmd.Command() {
	case "addsearch":
		b.addSearch(cmd.From)
	case "cancel":
		b.cancel(cmd.From)
	case "delsearch":
		b.delSearch(cmd.From)
	case "help":
		b.help(cmd.From)
	case "listsearch":
		b.listSearch(cmd.From)
	case "start":
		b.start(cmd.From)
	case "stop":
		b.stop(cmd.From)
	}
}

func (b *bot) cancel(u *tgbotapi.User) {
	_, existed := b.plaintextHandler[u.ID]
	delete(b.plaintextHandler, u.ID)
	if existed {
		b.sendMessage(u.ID, "Canceled.")
	} else {
		b.sendMessage(u.ID, "Nothing to cancel.")
	}
}

func (b *bot) help(u *tgbotapi.User) {
	b.sendHTMLMessage(u.ID, helpMsg)
}

func (b *bot) start(u *tgbotapi.User) {
	logger := log.WithFields(log.Fields{
		"func":     "start",
		"userID":   u.ID,
		"username": u.UserName,
	})

	user, err := b.db.GetUser(db.TelegramID(u.ID))
	if err != nil {
		logger.WithError(err).Error("Could not load user")
		b.alwaysSendMessage(u.ID, "Could not save start request, please try again later.")
		return
	}

	if user != nil {
		if user.Started {
			// Send them the message anyway in case they forgot they had already started the bot.
			b.sendMessage(u.ID, startedMsg)
			return
		}
		user.Started = true
	} else {
		user = &db.User{
			ID:       db.TelegramID(u.ID),
			Username: u.UserName,
			Started:  true,
		}
	}

	err = b.db.SaveUser(user)
	if err != nil {
		logger.WithError(err).Error("Could not save user")
		b.alwaysSendMessage(u.ID, "Could not save start request, please try again later.")
	}

	logger.Info("User started the bot")
	b.sendMessage(u.ID, startedMsg)
}

func (b *bot) stop(u *tgbotapi.User) {
	logger := log.WithFields(log.Fields{
		"func":     "stop",
		"userID":   u.ID,
		"username": u.UserName,
	})

	user, err := b.db.GetUser(db.TelegramID(u.ID))
	if err != nil {
		logger.WithError(err).Error("Could not load user")
		b.alwaysSendMessage(u.ID, "Could not save stop request, please try again later.")
		return
	}

	if user == nil {
		return
	}

	user.Started = false
	err = b.db.SaveUser(user)
	if err != nil {
		logger.WithError(err).Error("Could not save user")
		b.alwaysSendMessage(u.ID, "Could not save stop request, please try again later.")
	}

	logger.Info("User stopped the bot")
}
