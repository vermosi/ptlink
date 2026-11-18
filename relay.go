package main

// Supabase Realtime "broadcast" client (Phoenix channel protocol over WebSocket).

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Relay struct {
	ws     *wsConn
	topic  string
	ref    int64
	onMsg  func(json.RawMessage)
	joined chan error
	closed chan struct{}
	once   sync.Once
}

type phx struct {
	Topic   string          `json:"topic"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Ref     *string         `json:"ref"`
	JoinRef *string         `json:"join_ref,omitempty"`
}

func ConnectRelay(baseURL, apiKey, channel string, onMsg func(json.RawMessage)) (*Relay, error) {
	return ConnectRelayPresence(baseURL, apiKey, channel, "", onMsg)
}

// ConnectRelayPresence joins a channel; a non-empty presenceKey enables presence tracking.
func ConnectRelayPresence(baseURL, apiKey, channel, presenceKey string, onMsg func(json.RawMessage)) (*Relay, error) {
	u := strings.TrimRight(baseURL, "/")
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	u += "/realtime/v1/websocket?apikey=" + apiKey + "&vsn=1.0.0"
	ws, err := wsDial(u)
	if err != nil {
		return nil, err
	}
	r := &Relay{ws: ws, topic: "realtime:" + channel, onMsg: onMsg,
		joined: make(chan error, 1), closed: make(chan struct{})}
	go r.readLoop()
	go r.heartbeat()

	join := map[string]any{
		"config": map[string]any{
			"broadcast":        map[string]any{"ack": false, "self": false},
			"presence":         map[string]any{"key": presenceKey, "enabled": presenceKey != ""},
			"postgres_changes": []any{},
			"private":          false,
		},
	}
	if strings.HasPrefix(apiKey, "eyJ") { // legacy JWT anon keys double as access tokens
		join["access_token"] = apiKey
	}
	if err := r.send(r.topic, "phx_join", join, true); err != nil {
		ws.Close()
		return nil, err
	}
	select {
	case err := <-r.joined:
		if err != nil {
			ws.Close()
			return nil, err
		}
	case <-time.After(20 * time.Second):
		ws.Close()
		return nil, fmt.Errorf("timed out joining relay channel")
	}
	return r, nil
}

func (r *Relay) nextRef() string { return strconv.FormatInt(atomic.AddInt64(&r.ref, 1), 10) }

func (r *Relay) send(topic, event string, payload any, isJoin bool) error {
	ref := r.nextRef()
	m := map[string]any{"topic": topic, "event": event, "payload": payload, "ref": ref}
	if topic != "phoenix" {
		m["join_ref"] = "1"
	}
	if isJoin {
		m["ref"] = "1"
		atomic.StoreInt64(&r.ref, 1)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return r.ws.WriteText(b)
}

// Broadcast sends one message to everyone else on the channel.
func (r *Relay) Broadcast(payload any) error {
	return r.send(r.topic, "broadcast", map[string]any{
		"type": "broadcast", "event": "m", "payload": payload}, false)
}

// Track publishes (or updates) this client's presence state on the channel.
func (r *Relay) Track(state any) error {
	return r.send(r.topic, "presence", map[string]any{
		"type": "presence", "event": "track", "payload": state}, false)
}

func (r *Relay) heartbeat() {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.send("phoenix", "heartbeat", map[string]any{}, false)
		case <-r.closed:
			return
		}
	}
}

func (r *Relay) readLoop() {
	defer r.Close()
	for {
		b, err := r.ws.ReadMessage()
		if err != nil {
			log.Printf("relay connection lost: %v", err)
			select {
			case r.joined <- err:
			default:
			}
			return
		}
		var m phx
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		switch m.Event {
		case "phx_reply":
			if m.Topic != r.topic || m.Ref == nil || *m.Ref != "1" {
				continue
			}
			var rep struct {
				Status   string          `json:"status"`
				Response json.RawMessage `json:"response"`
			}
			json.Unmarshal(m.Payload, &rep)
			var e error
			if rep.Status != "ok" {
				e = fmt.Errorf("relay refused join: %s %s", rep.Status, string(rep.Response))
			}
			select {
			case r.joined <- e:
			default:
			}
		case "broadcast":
			var bp struct {
				Event   string          `json:"event"`
				Payload json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(m.Payload, &bp) == nil && bp.Event == "m" {
				r.onMsg(bp.Payload)
			}
		case "phx_error", "system":
			log.Printf("relay says: %s %s", m.Event, string(m.Payload))
		}
	}
}

func (r *Relay) Close() {
	r.once.Do(func() { close(r.closed); r.ws.Close() })
}

func (r *Relay) Done() <-chan struct{} { return r.closed }
