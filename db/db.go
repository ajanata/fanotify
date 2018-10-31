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

package db

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/etcd-io/bbolt"
)

var (
	versionKey = []byte("version")
	version1   = []byte("1")

	metadataBucket = []byte("metadata")
	searchesBucket = []byte("searches")
	faUsersBucket  = []byte("fa_users")
	tgUsersBucket  = []byte("tg_users")

	ErrCannotSaveNonIteration = errors.New("cannot save non-iteration item")
	ErrNoTGUser               = errors.New("no such telegram user")
)

type (
	// TelegramID is the type of Telegram entity IDs.
	TelegramID int64

	// UserLoader is used to load a user while iterating over saved items.
	UserLoader func(id TelegramID) (*TGUser, error)

	SearchIterator func(search *Search, ul UserLoader) error
	UserIterator   func(faUser *FAUser, ul UserLoader) error

	// DB is an interface that can load and store information in a database.
	DB interface {
		Close() error

		AddSearchForUser(userID TelegramID, search string) error
		DeleteSearchForUser(userID TelegramID, search string) error
		IterateSearches(cb SearchIterator) error

		AddUserSubmissionsForUser(userID TelegramID, faUser string) error
		DeleteUserSubmissionsForUser(userID TelegramID, faUser string) error
		AddUserJournalsForUser(userID TelegramID, faUser string) error
		DeleteUserJournalsForUser(userID TelegramID, faUser string) error
		IterateUsers(cb UserIterator) error

		GetTGUser(id TelegramID) (*TGUser, error)
		SaveTGUser(user *TGUser) error
	}

	db struct {
		b *bolt.DB
	}
)

// New creates a new database connection.
func New(filename string) (DB, error) {
	b, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	err = b.Update(func(tx *bolt.Tx) error {
		m, err := tx.CreateBucketIfNotExists(metadataBucket)
		if err != nil {
			return fmt.Errorf("create metadata bucket: %s", err)
		}
		v := m.Get(versionKey)
		if v == nil {
			if err := m.Put(versionKey, version1); err != nil {
				return fmt.Errorf("save version: %s", err)
			}
		} else if len(v) != len(version1) || v[0] != version1[0] {
			// TODO better check once there are more version
			return fmt.Errorf("bad db version: %s", v)
		}

		_, err = tx.CreateBucketIfNotExists(searchesBucket)
		if err != nil {
			return fmt.Errorf("create searches bucket: %s", err)
		}

		_, err = tx.CreateBucketIfNotExists(faUsersBucket)
		if err != nil {
			return fmt.Errorf("create furaffinity users bucket: %s", err)
		}

		_, err = tx.CreateBucketIfNotExists(tgUsersBucket)
		if err != nil {
			return fmt.Errorf("create telegram users bucket: %s", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &db{
		b: b,
	}, nil
}

func (d *db) Close() error {
	return d.b.Close()
}

func (id TelegramID) Key() []byte {
	return []byte(strconv.FormatInt(int64(id), 10))
}
