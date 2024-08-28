/*
midgaard_bot, a Telegram bot which sets a bridge to Midgaard Merc MUD
Copyright (C) 2017 by Javier Sancho Fernandez <jsf at jsancho dot org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/json"
	"log"
)

type Session struct {
	Channels    []*WsChannel
	SessionId   string
	SessionKey  string
	SessionData *SessionData
}

type SessionData struct {
	SongPath  string `json:"songPath,omitempty"`
	Transpose int    `json:"transpose"`
	Section   string `json:"section,omitempty"`
}

type SessionDataMsg struct {
	SessionData
	ClientKey string `json:"clientKey,omitempty"`
}

var sessions map[string]*Session

func initSessions() {
	sessions = make(map[string]*Session)
}

func registerWs(wc *WsChannel) {
	log.Default().Printf("registered ws for %s", wc.SessionId)
	session, ok := sessions[wc.SessionId]
	if !ok {
		session = &Session{
			SessionId:   wc.SessionId,
			SessionKey:  "",
			Channels:    make([]*WsChannel, 0),
			SessionData: &SessionData{},
		}
		sessions[wc.SessionId] = session
	}
	session.Channels = append(session.Channels, wc)
}

func sendUpdateTo(wc *WsChannel) {
	session, ok := sessions[wc.SessionId]
	if !ok {
		return
	}

	sent, err := json.Marshal(session.SessionData)
	sents := string(sent)
	if err != nil {
		log.Default().Printf("registering ws: %s", err.Error())
		return
	}
	wc.SendChannel <- &sents
}

func deregisterWs(wc *WsChannel) {
	session, ok := sessions[wc.SessionId]
	if !ok {
		return
	}
	i := 0
	for idx, nwc := range session.Channels {
		if wc != nwc {
			session.Channels[i] = session.Channels[idx]
			i++
		}
	}
	if i == 0 {
		log.Default().Printf("Deleting session %s", wc.SessionId)
		delete(sessions, wc.SessionId)
	}
	session.Channels = session.Channels[:i]
}

func processMessage(sender *WsChannel, message *string) {
	session, ok := sessions[sender.SessionId]
	if !ok {
		sender.CancelFunc()
		return
	}
	messageData := &SessionDataMsg{}
	err := json.Unmarshal([]byte(*message), messageData)
	if err != nil {
		log.Default().Printf("processing message: %s", err.Error())
		return
	}

	if session.SessionKey == "" && messageData.ClientKey != "" {
		// claim session
		session.SessionKey = messageData.ClientKey
	}

	hasKey := messageData.ClientKey == session.SessionKey

	if messageData.Section != "" {
		session.SessionData.Section = messageData.Section
	}

	if hasKey {
		if messageData.SongPath != "" {
			session.SessionData.SongPath = messageData.SongPath
		}
		session.SessionData.Transpose = messageData.Transpose
		sent, err := json.Marshal(session.SessionData)
		sents := string(sent)
		if err != nil {
			log.Default().Printf("processing message: %s", err.Error())
			return
		}
		for _, wc := range session.Channels {
			wc.SendChannel <- &sents
		}
	}
}
