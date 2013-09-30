package main

import (
	"io"
	"log"
	"net/http"
	"sync"

	"code.google.com/p/go.net/websocket"
	"github.com/sdgoij/gobbb"
)

func HandleConnect(c *Client, event WsEvent) error { return nil }
func HandleCreate(c *Client, event WsEvent) error  { return nil }
func HandleJoinURL(c *Client, event WsEvent) error { return nil }
func HandleEnd(c *Client, event WsEvent) error     { return nil }

var handler *WsEventHandler = &WsEventHandler{
	h: map[string]WsEventHandlerFunc{
		"connect": HandleConnect,
		"create":  HandleCreate,
		"joinURL": HandleJoinURL,
		"end":     HandleEnd,
	},
	c: map[*Client]struct{}{},
}

func init() {
	http.Handle("/ws", websocket.Server{Handler: HandleWS})
}

func HandleWS(ws *websocket.Conn) {
	remoteAddr := ws.Request().RemoteAddr
	log.Printf("Connection from %s opened", remoteAddr)

	client := &Client{
		address: remoteAddr,
		conn:    ws,
		done:    make(chan struct{}),
		events:  make(chan WsEvent),
	}

	handler.AddClient(client)

	defer func() {
		log.Println("Connection from %s closed", remoteAddr)
		handler.RemoveClient(client)
	}()

	go client.Writer()
	client.Reader()
}

type Client struct {
	address string
	conn    *websocket.Conn
	b3      bbb.BigBlueButton
	done    chan struct{}
	events  chan WsEvent
	handler *WsEventHandler

	Id string
}

func (c *Client) Reader() {
	for {
		var ev WsEvent
		if err := websocket.JSON.Receive(c.conn, &ev); nil != err {
			if io.EOF == err {
				log.Printf("Reader[%s]: %s", c.address, err)
				c.done <- struct{}{}
				return
			}
		}
		if err := c.handler.Handle(c, ev); nil != err {
			log.Printf("Reader[%s]: %s", c.address, err)
		}
	}
}

func (c *Client) Writer() {
	for {
		select {
		case e := <-c.events:
			log.Printf("Writer[%s]: %#v", c.address, e)
			if err := websocket.JSON.Send(c.conn, e); nil != err {
				log.Printf("Writer[%s]: %s", c.address, err)
			}
		case <-c.done:
			log.Printf("Writer[%s]: exit", c.address)
			return
		}
	}
}

type WsEventData map[string]interface{}

type WsEvent struct {
	Event string      `json:"event"`
	Data  WsEventData `json:"data"`
}

type WsEventHandlerFunc func(*Client, WsEvent) error

type WsEventHandler struct {
	h map[string]WsEventHandlerFunc
	c map[*Client]struct{}
	m sync.RWMutex
}

func (ws *WsEventHandler) Handle(c *Client, ev WsEvent) error {
	if h, t := ws.h[ev.Event]; t {
		return h(c, ev)
	}
	return newWsEventHandlerNotFound(ev.Event)
}

func (ws *WsEventHandler) AddClient(c *Client) {
	ws.m.Lock()
	defer ws.m.Unlock()
	if _, t := ws.c[c]; !t {
		ws.c[c] = struct{}{}
		c.handler = ws
	}
}

func (ws *WsEventHandler) RemoveClient(c *Client) {
	ws.m.Lock()
	defer ws.m.Unlock()
	if _, t := ws.c[c]; t {
		delete(ws.c, c)
		c.handler = nil
	}
}

func (ws *WsEventHandler) Broadcast(event WsEvent) error {
	ws.m.RLock()
	defer ws.m.RUnlock()
	for peer, _ := range ws.c {
		peer.events <- event
	}
	return nil
}

type WsEventHandlerNotFound string

func (e WsEventHandlerNotFound) Error() string {
	return "Event Handler '" + string(e) + "' not found!"
}

func newWsEventHandlerNotFound(e string) WsEventHandlerNotFound {
	return WsEventHandlerNotFound(e)
}