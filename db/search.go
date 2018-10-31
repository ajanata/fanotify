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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/etcd-io/bbolt"
)

type (
	Search struct {
		// Set during iteration to allow the search to be saved.
		tx      *bolt.Tx
		Search  string              `json:"search"`
		LastRun time.Time           `json:"last_run"`
		LastID  int64               `json:"last_id"`
		Users   map[TelegramID]bool `json:"tg_users"`
	}
)

var (
	ErrNoSearch = errors.New("no such search")
)

func (d *db) AddSearchForUser(userID TelegramID, search string) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		// Add the user to the search, creating it if needed.
		so, err := getSearch(search, tx)
		if err != nil {
			return err
		}

		if so == nil {
			so = &Search{
				Search:  search,
				LastRun: time.Unix(0, 0),
				LastID:  0,
				Users:   map[TelegramID]bool{},
			}
		}

		so.Users[userID] = true
		err = saveSearch(so, tx)
		if err != nil {
			return err
		}

		// Add the search to the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.Searches == nil {
			user.Searches = make(map[string]bool)
		}
		user.Searches[search] = true
		return saveTGUser(user, tx)
	})
}

// getSearch is a helper func to load a search from the DB for a given search string.
// Returns nil if the search does not exist.
func getSearch(search string, tx *bolt.Tx) (*Search, error) {
	b := tx.Bucket(searchesBucket)
	if b == nil {
		return nil, errors.New("could not load searches bucket")
	}

	data := b.Get([]byte(search))
	if data == nil {
		return nil, nil
	}
	s := &Search{}
	err := json.Unmarshal(data, s)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling search: %s", err)
	}
	return s, nil
}

func saveSearch(search *Search, tx *bolt.Tx) error {
	b := tx.Bucket(searchesBucket)
	if b == nil {
		return errors.New("could not load searches bucket")
	}

	data, err := json.Marshal(search)
	if err != nil {
		return fmt.Errorf("marshalling search: %s", err)
	}

	return b.Put([]byte(search.Search), data)
}

func (d *db) DeleteSearchForUser(userID TelegramID, search string) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		// Delete the user from the search.
		so, err := getSearch(search, tx)
		if err != nil {
			return err
		}
		if so == nil {
			return ErrNoSearch
		}
		existed := so.Users[userID]

		delete(so.Users, userID)
		if len(so.Users) == 0 {
			b := tx.Bucket(searchesBucket)
			if b == nil {
				return errors.New("could not load searches bucket")
			}
			err = b.Delete([]byte(search))
		} else {
			err = saveSearch(so, tx)
		}
		if err != nil {
			return err
		}

		// Delete the search from the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.Searches == nil {
			user.Searches = make(map[string]bool)
		}
		existed = existed && user.Searches[search]
		delete(user.Searches, search)
		err = saveTGUser(user, tx)
		if err != nil {
			return err
		}

		if !existed {
			return ErrNoSearch
		}
		return nil
	})
}

// Update saves the current state of the search back to the database, if the search was loaded via iteration.
// Otherwise, ErrCannotSaveNonIteration is returned.
func (s *Search) Update() error {
	if s.tx == nil {
		return ErrCannotSaveNonIteration
	}

	return saveSearch(s, s.tx)
}

func (d *db) IterateSearches(cb SearchIterator) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(searchesBucket)
		if b == nil {
			return errors.New("could not load searches bucket")
		}

		ul := func(id TelegramID) (*TGUser, error) {
			return getTGUser(id, tx)
		}

		return b.ForEach(func(k, v []byte) error {
			s := &Search{}
			err := json.Unmarshal(v, s)
			if err != nil {
				return fmt.Errorf("unmarshalling search: %s", err)
			}
			s.tx = tx

			// TODO maybe we shouldn't immediately return the error from the callback?
			// If it's a transient FA error, we probably should keep trying the rest.
			return cb(s, ul)
		})
	})
}
