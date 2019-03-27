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
	"strings"
	"time"

	"github.com/etcd-io/bbolt"
)

type (
	FAUser struct {
		// Set during iteration to allow the user to be saved.
		tx *bolt.Tx
		// TODO have a way to get and store the preferred case of the username
		Username         string              `json:"username"`
		LastRun          time.Time           `json:"last_run"`
		LastSubmissionID int64               `json:"last_submission_id"`
		LastJournalID    int64               `json:"last_journal_id"`
		SubmissionUsers  map[TelegramID]bool `json:"submission_users"`
		JournalUsers     map[TelegramID]bool `json:"journal_users"`
	}
)

var (
	ErrNoFAUser = errors.New("no such furaffinity user")
)

func (d *db) AddUserSubmissionsForUser(userID TelegramID, faUser string) error {
	faUser = strings.ToLower(faUser)
	return d.b.Update(func(tx *bolt.Tx) error {
		// Add the user to the fa user, creating it if needed.
		fa, err := getFAUser(faUser, tx)
		if err != nil {
			return err
		}

		if fa == nil {
			fa = &FAUser{
				Username:         faUser,
				LastRun:          time.Unix(0, 0),
				LastJournalID:    0,
				LastSubmissionID: 0,
				JournalUsers:     map[TelegramID]bool{},
				SubmissionUsers:  map[TelegramID]bool{},
			}
		}

		fa.SubmissionUsers[userID] = true
		err = saveFAUser(fa, tx)
		if err != nil {
			return err
		}

		// Add the fa user to the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.SubmissionUsers == nil {
			user.SubmissionUsers = make(map[string]bool)
		}
		user.SubmissionUsers[faUser] = true
		return saveTGUser(user, tx)
	})
}

func (d *db) DeleteUserSubmissionsForUser(userID TelegramID, faUser string) error {
	faUser = strings.ToLower(faUser)
	return d.b.Update(func(tx *bolt.Tx) error {
		// Delete the user from the fa user.
		fa, err := getFAUser(faUser, tx)
		if err != nil {
			return err
		}
		if fa == nil {
			return ErrNoFAUser
		}
		existed := fa.SubmissionUsers[userID]

		delete(fa.SubmissionUsers, userID)
		if len(fa.SubmissionUsers) == 0 {
			b := tx.Bucket(faUsersBucket)
			if b == nil {
				return errors.New("could not load furaffinity users bucket")
			}
			err = b.Delete([]byte(faUser))
		} else {
			err = saveFAUser(fa, tx)
		}
		if err != nil {
			return err
		}

		// Delete the fa user from the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.SubmissionUsers == nil {
			user.SubmissionUsers = make(map[string]bool)
		}
		existed = existed && user.SubmissionUsers[faUser]
		delete(user.SubmissionUsers, faUser)
		err = saveTGUser(user, tx)
		if err != nil {
			return err
		}

		if !existed {
			return ErrNoFAUser
		}
		return nil
	})
}

func (d *db) AddUserJournalsForUser(userID TelegramID, faUser string) error {
	faUser = strings.ToLower(faUser)
	return d.b.Update(func(tx *bolt.Tx) error {
		// Add the user to the fa user, creating it if needed.
		fa, err := getFAUser(faUser, tx)
		if err != nil {
			return err
		}

		if fa == nil {
			fa = &FAUser{
				Username:         faUser,
				LastRun:          time.Unix(0, 0),
				LastJournalID:    0,
				LastSubmissionID: 0,
				JournalUsers:     map[TelegramID]bool{},
				SubmissionUsers:  map[TelegramID]bool{},
			}
		}

		fa.JournalUsers[userID] = true
		err = saveFAUser(fa, tx)
		if err != nil {
			return err
		}

		// Add the fa user to the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.JournalUsers == nil {
			user.JournalUsers = make(map[string]bool)
		}
		user.JournalUsers[faUser] = true
		return saveTGUser(user, tx)
	})
}

func (d *db) DeleteUserJournalsForUser(userID TelegramID, faUser string) error {
	faUser = strings.ToLower(faUser)
	return d.b.Update(func(tx *bolt.Tx) error {
		// Delete the user from the fa user.
		fa, err := getFAUser(faUser, tx)
		if err != nil {
			return err
		}
		if fa == nil {
			return ErrNoFAUser
		}
		existed := fa.JournalUsers[userID]

		delete(fa.JournalUsers, userID)
		if len(fa.JournalUsers) == 0 {
			b := tx.Bucket(faUsersBucket)
			if b == nil {
				return errors.New("could not load furaffinity users bucket")
			}
			err = b.Delete([]byte(faUser))
		} else {
			err = saveFAUser(fa, tx)
		}
		if err != nil {
			return err
		}

		// Delete the fa user from the user.
		user, err := getTGUser(userID, tx)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNoTGUser
		}
		if user.JournalUsers == nil {
			user.JournalUsers = make(map[string]bool)
		}
		existed = existed && user.JournalUsers[faUser]
		delete(user.JournalUsers, faUser)
		err = saveTGUser(user, tx)
		if err != nil {
			return err
		}

		if !existed {
			return ErrNoFAUser
		}
		return nil
	})
}

func (d *db) IterateFAUsers(cb UserIterator) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(faUsersBucket)
		if b == nil {
			return errors.New("could not load furaffinity users bucket")
		}

		ul := func(id TelegramID) (*TGUser, error) {
			return getTGUser(id, tx)
		}

		return b.ForEach(func(k, v []byte) error {
			fa := &FAUser{}
			err := json.Unmarshal(v, fa)
			if err != nil {
				return fmt.Errorf("unmarshalling furaffinity user: %s", err)
			}
			fa.tx = tx

			// TODO maybe we shouldn't immediately return the error from the callback?
			// If it's a transient FA error, we probably should keep trying the rest.
			return cb(fa, ul)
		})
	})
}

func getFAUser(user string, tx *bolt.Tx) (*FAUser, error) {
	b := tx.Bucket(faUsersBucket)
	if b == nil {
		return nil, errors.New("could not load furaffinity users bucket")
	}

	data := b.Get([]byte(user))
	if data == nil {
		return nil, nil
	}
	u := &FAUser{}
	err := json.Unmarshal(data, u)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling furaffinity user: %s", err)
	}
	return u, nil
}

func saveFAUser(user *FAUser, tx *bolt.Tx) error {
	b := tx.Bucket(faUsersBucket)
	if b == nil {
		return errors.New("could not load furaffinity users bucket")
	}

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshalling furaffinity user: %s", err)
	}

	return b.Put([]byte(user.Username), data)
}

// Update saves the current state of the user back to the database, if the user was loaded via iteration.
// Otherwise, ErrCannotSaveNonIteration is returned.
func (u *FAUser) Update() error {
	if u.tx == nil {
		return ErrCannotSaveNonIteration
	}

	return saveFAUser(u, u.tx)
}
