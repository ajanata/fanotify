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
	"time"

	"github.com/ajanata/faapi"
	"github.com/koding/multiconfig"
)

type (
	// Config is the configuration for the bot.
	Config struct {
		Debug    bool   `default:"false"`
		LogLevel string `default:"INFO"`
		DB       DB
		FA       FA
		TG       TG
	}

	// DB is the database configuration.
	DB struct {
		File string `default:"fanotify.bolt"`
	}

	// TG is the configuration for Telegram.
	TG struct {
		Token   string `required:"true"`
		OwnerID string `required:"true"`
	}

	// FA is the configuration for FurAffinity.
	FA struct {
		Cookies   []Cookie
		Proxy     string
		RateLimit duration `default:"10s"`
		UserAgent string   `required:"true"`
	}

	// Cookie is an HTTP cookie.
	Cookie struct {
		Name  string
		Value string
	}

	duration struct {
		time.Duration
	}
)

func loadConfig() *Config {
	m := multiconfig.NewWithPath("fanotify.toml")
	c := new(Config)
	m.MustLoad(c)
	return c
}

func (d *duration) UnmarshalText(text []byte) (err error) {
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func (c *FA) faAPIConfig() faapi.Config {
	cookies := make([]faapi.Cookie, len(c.Cookies))
	for i, c := range cookies {
		cookies[i] = faapi.Cookie{
			Name:  c.Name,
			Value: c.Value,
		}
	}
	// this is so dumb
	rl, err := time.ParseDuration(c.RateLimit.String())
	if err != nil {
		panic(err)
	}
	return faapi.Config{
		Cookies:   cookies,
		Proxy:     c.Proxy,
		RateLimit: rl,
		UserAgent: c.UserAgent,
	}
}