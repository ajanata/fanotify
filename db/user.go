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
	// User represents a telegram user in the database.
	User struct {
		Name          string     `json:"name"`
		Started       bool       `json:"started"`
		ID            TelegramID `json:"id"`
		LastMessage   time.Time  `json:"last_message"`
		AlertKeywords []string   `json:"alert_keywords"`
	}
)

// GetUser loads the user with the given ID, if the user exists. If the user
// does not exist, nil is returned.
func (d *db) GetUser(id TelegramID) (*User, error) {
	var user *User
	err := d.b.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return errors.New("could not load users bucket")
		}

		data := b.Get(id.Key())
		if data == nil {
			return nil
		}
		user = &User{}

		err := json.Unmarshal(data, user)
		if err != nil {
			return fmt.Errorf("unmarshalling user: %s", err)
		}

		return nil
	})
	return user, err
}

// SaveUser saves the given user in the database, overwriting any old information about the user.
func (d *db) SaveUser(user *User) error {
	return d.b.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(usersBucket)
		if b == nil {
			return errors.New("could not load users bucket")
		}

		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshalling user: %s", err)
		}

		return b.Put(user.ID.Key(), data)
	})
}
