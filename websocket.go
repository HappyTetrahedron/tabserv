/*
tabserv, a websocket shared state server

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
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type WsConfig struct {
	Address string `short:"a" long:"address" description:"Local address at which to bind the websocket server" required:"true"`
}

type WsChannel struct {
	SendChannel chan *string
	CancelFunc  context.CancelFunc
	SessionId   string
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 8192

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Time to wait before force close on connection.
	closeGracePeriod = 10 * time.Second
)

func receiveWorker(ws *websocket.Conn, wsChan *WsChannel) {
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println("receive error:", wsChan.SessionId, err)
			deregisterWs(wsChan)
			break
		}
		msg := string(message)
		processMessage(wsChan, &msg)
	}
	cleanupWs(wsChan)
}

func sendWorker(sendChannel chan *string, ws *websocket.Conn, ctx context.Context) {
	for {
		select {
		case msg := <-sendChannel:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.TextMessage, []byte(*msg)); err != nil {
				log.Println("write error:", err)
			}
		case <-ctx.Done():
			log.Default().Printf("Closing send channel")
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			time.Sleep(closeGracePeriod)
			ws.Close()
			return
		}
	}
}

func ping(ws *websocket.Conn, ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait)); err != nil {
				log.Println("ping:", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

var upgrader = websocket.Upgrader{}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("error during upgrade:", err)
		return
	}

	vars := mux.Vars(r)
	sessionId := vars["session"]

	sendChannel := make(chan *string)
	ctx, cancel := context.WithCancel(context.Background())
	wc := &WsChannel{
		SendChannel: sendChannel,
		CancelFunc:  cancel,
		SessionId:   sessionId,
	}

	registerWs(wc)

	go receiveWorker(ws, wc)
	go sendWorker(sendChannel, ws, ctx)
	go ping(ws, ctx)

	sendUpdateTo(wc)
}

func cleanupWs(ch *WsChannel) {
	deregisterWs(ch)
	ch.CancelFunc()
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do stuff here
		log.Println(r.RequestURI)
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func initWebsockets(config WsConfig) error {
	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.HandleFunc("/{session}", serveWs)
	r.NotFoundHandler = r.NewRoute().HandlerFunc(http.NotFound).GetHandler()
	server := &http.Server{
		Addr:              config.Address,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           r,
	}
	log.Fatal(server.ListenAndServe())

	return nil
}
