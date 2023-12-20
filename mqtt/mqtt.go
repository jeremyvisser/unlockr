package mqtt

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/go-mqtt/mqtt"
	"jeremy.visser.name/unlockr/device"
)

const Timeout = 5 * time.Second
const KeepAlive = 300 // seconds
const BufLen = 64     // messages

const statusOnline = "Online"
const statusOffline = "Offline"

type Payload string
type Topic string

type Message struct {
	Payload `json:"message"`
	Topic   `json:"topic"`
}

type Mqtt struct {
	// Network is optional, and defaults to "tcp" if empty.
	Network string

	// Address is the network address of the MQTT server. Required.
	Address string

	// Username and Password as per server requirements.
	// Empty string means authentication is not attempted.
	Username, Password string

	// TLS is optional. A value of nil means TLS is not used.
	TLS *tls.Config

	// Uniquely identifies the client for session resumption. Optional.
	// Defaults to hostname if unset.
	ClientID string

	c   *mqtt.Client
	cmu sync.Mutex
	rmu sync.Mutex

	subs   listenGroup[Message]
	topics map[Topic]struct{}
}

func (m *Mqtt) clientID() string {
	if m.ClientID != "" {
		return m.ClientID
	}
	if h, err := os.Hostname(); h != "" && err == nil {
		return h
	}
	return "unlockr"
}

func (m *Mqtt) statusTopic() string {
	return fmt.Sprintf("unlockr/%s/status", m.clientID())
}

func (m *Mqtt) readLoop(c *mqtt.Client) {
	if !m.rmu.TryLock() {
		return // readLoop already running
	}
	go func() {
		defer m.rmu.Unlock()
		pub := make(chan Message, BufLen)
		defer close(pub)
		go m.subs.publish(pub)
		for {
			payload, topic, err := c.ReadSlices()
			if err != nil {
				log.Printf("Mqtt ReadSlices: %v", err)
				return
			}
			log.Printf("Mqtt <- %s = %s", topic, payload)
			select {
			case pub <- Message{Payload(payload), Topic(topic)}:
				continue
			default:
				log.Printf("Mqtt: discarded 1 message due to full buffer (%d messages queued)", BufLen)
			}
		}
	}()
	c.PublishRetained(nil, []byte(statusOnline), m.statusTopic())
}

func (m *Mqtt) client() (*mqtt.Client, error) {
	m.cmu.Lock()
	defer m.cmu.Unlock()
	if m.c == nil {
		if c, err := mqtt.VolatileSession(m.clientID(), m.config()); err != nil {
			return nil, err
		} else {
			m.c = c
		}
	}
	m.readLoop(m.c)
	return m.c, nil
}

func (m *Mqtt) config() *mqtt.Config {
	return &mqtt.Config{
		Dialer:       m.dialer(),
		PauseTimeout: Timeout,
		KeepAlive:    KeepAlive,
		UserName:     m.Username,
		Password:     []byte(m.Password),
		CleanSession: false,
		Will: struct {
			Topic       string
			Message     []byte
			Retain      bool
			AtLeastOnce bool
			ExactlyOnce bool
		}{
			Topic:   m.statusTopic(),
			Message: []byte(statusOffline),
			Retain:  true,
		},
	}
}

func (m *Mqtt) dialer() mqtt.Dialer {
	network := "tcp"
	if m.Network != "" {
		network = m.Network
	}
	if m.TLS != nil {
		return mqtt.NewTLSDialer(network, m.Address, m.TLS)
	} else {
		return mqtt.NewDialer(network, m.Address)
	}
}

func (m *Mqtt) publish(quit <-chan struct{}, message *Message) error {
	if c, err := m.client(); err != nil {
		return err
	} else {
		log.Printf("Mqtt -> %s = %s", message.Topic, message.Payload)
		return c.Publish(quit, []byte(message.Payload), string(message.Topic))
	}
}

// subscribe will create an MQTT subscription to topicFilter.
// Multiple calls with the same topicFilter results in one subscription.
//
// Each caller must pass a quit channel. If quit is nil, it blocks forever.
// When finished, closing quit will cause msgs to be closed, but must be read
// from to clear the backlog.
func (m *Mqtt) subscribe(quit <-chan struct{}, topicFilter Topic) (msgs <-chan Message, err error) {
	c, err := m.client()
	if err != nil {
		return nil, err
	}
	if _, ok := m.topics[topicFilter]; !ok {
		if err := c.Subscribe(quit, string(topicFilter)); err != nil {
			log.Printf("Mqtt.subscribe: %v", err)
			return nil, err
		}
	}
	msgs = m.subs.subscribe(quit)
	return msgs, nil
}

var DefaultMqtt Mqtt

type Expect struct {
	Send *Message
	Recv *Message
}

var ErrExpectTimeout = errors.New("expect: timeout waiting for message")

func (e *Expect) Run(ctx context.Context, mq *Mqtt) (err error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	if e.Send == nil {
		return errors.New("send is a required parameter")
	}
	var msgs <-chan Message
	if e.Recv != nil && e.Recv.Topic != "" {
		msgs, err = mq.subscribe(ctx.Done(), e.Recv.Topic)
		if err != nil {
			return err
		}
	}
	if err := mq.publish(ctx.Done(), e.Send); err != nil {
		return err
	}
	if msgs != nil {
		err := ErrExpectTimeout
		for m := range msgs {
			if m.Topic == e.Recv.Topic {
				if e.Recv.Payload == "" || m.Payload == e.Recv.Payload {
					err = nil
					cancel()
					// set success and close channel, but loop to clear backlog
				}
			}
		}
		return err
	}
	return nil
}

type Device struct {
	device.Base
	PowerCmd *Expect
	*Mqtt    `json:"-"`
}

func (d *Device) mqtt() *Mqtt {
	if d.Mqtt == nil {
		return &DefaultMqtt
	}
	return d.Mqtt
}

func (d *Device) Power(ctx context.Context, on bool) (err error) {
	defer func() {
		if err != nil {
			log.Printf("Mqtt[%s] power on=%v; error: %v", d.GetName(), on, err)
		}
		log.Printf("Mqtt[%s] power on=%v; success", d.GetName(), on)
	}()
	return d.PowerCmd.Run(ctx, d.mqtt())
}

type listenGroup[T any] struct {
	m  map[chan T]struct{}
	mu sync.Mutex
}

// subscribe returns a channel, from which published messages can be read.
// Listeners can unsubscribe by closing quit. After closing quit, msgsR must
// continue to be read until closed.
func (g *listenGroup[T]) subscribe(quit <-chan struct{}) (msgsR <-chan T) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = make(map[chan T]struct{})
	}
	msgs := make(chan T)
	g.m[msgs] = struct{}{} // append new chan to group
	go func() {
		<-quit // block until done
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.m, msgs)
		close(msgs)
	}()
	return msgs
}

// publish accepts a channel, and consumes messages until it is closed.
// Each subscriber receives a copy of the message.
func (g *listenGroup[T]) publish(messages <-chan T) {
	for msg := range messages {
		g.mu.Lock()
		for v := range g.m {
			v <- msg
		}
		g.mu.Unlock()
	}
}
