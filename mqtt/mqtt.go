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

type Message struct {
	Message string
	Topic   string
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

	subs listenGroup[Message]
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
			message, topic, err := c.ReadSlices()
			if err != nil {
				log.Printf("Mqtt ReadSlices: %v", err)
				return
			}
			log.Printf("Mqtt <- %s = %s", topic, message)
			select {
			case pub <- Message{string(message), string(topic)}:
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

func (m *Mqtt) publish(quit <-chan struct{}, message, topic string) error {
	if c, err := m.client(); err != nil {
		return err
	} else {
		log.Printf("Mqtt -> %s = %s", topic, message)
		return c.Publish(quit, []byte(message), topic)
	}
}

// subscribe will create an MQTT subscription to topicFilter.
// Multiple calls with the same topicFilter results in one subscription.
//
// Each caller must pass a quit channel and close when done.
// Context.Done() can be passed as the quit channel.
// When the last caller quits, the topic is unsubscribed.
func (m *Mqtt) subscribe(quit <-chan struct{}, topicFilter string) (msgs <-chan Message, err error) {
	c, err := m.client()
	if err != nil {
		return nil, err
	}
	start := func() error {
		return c.Subscribe(quit, topicFilter)
	}
	finish := func() {
		// Best-effort unsubscribe: we're unwilling to wait more than Timeout.
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		if err := c.Unsubscribe(ctx.Done(), topicFilter); err != nil {
			log.Printf("Mqtt.subscribe: finish unsubscribe failed: %v", err)
		} else if err := ctx.Err(); err != nil {
			log.Printf("Mqtt.subscribe: finish context failed: %v", err)
		}
	}
	msgs, done, err := m.subs.subscribe(topicFilter, start, finish)
	go func() {
		<-quit // wait for context to cancel
		done()
	}()
	return msgs, err
}

var DefaultMqtt Mqtt

type Cmd struct {
	Topic, Message string
}

type Expect struct {
	Send *Cmd
	Recv *Cmd
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
	if err := mq.publish(ctx.Done(), e.Send.Message, e.Send.Topic); err != nil {
		return err
	}
	if msgs != nil {
		err := ErrExpectTimeout
		for m := range msgs {
			if m.Topic == e.Recv.Topic {
				if e.Recv.Message == "" || m.Message == e.Recv.Message {
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
	m  map[string]map[chan T]struct{}
	mu sync.Mutex
}

// subscribe takes a topic as key, and calls start() for the first listener.
// Messages sent to publish() are written to msgsR. When finished, call done().
//
// When the last listener calls done(), finish() is called.
// If start() errors, msgsR and done are nil, and finish is not called.
func (g *listenGroup[T]) subscribe(key string, start func() error, finish func()) (msgsR <-chan T, done func(), err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = make(map[string]map[chan T]struct{})
	}
	if _, ok := g.m[key]; !ok {
		if err := start(); err != nil {
			return nil, nil, err
		}
		newgrp := make(map[chan T]struct{})
		g.m[key] = newgrp
	}
	msgsW := make(chan T)
	g.m[key][msgsW] = struct{}{}
	msgsR = msgsW
	done = func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.m[key], msgsW)
		close(msgsW)
		if len(g.m[key]) == 0 {
			finish()
			delete(g.m, key)
		}
	}
	return msgsR, done, nil
}

// publish accepts a channel, and consumes messages until it is closed.
// Each subscriber receives a copy of the message.
func (g *listenGroup[T]) publish(messages <-chan T) {
	for msg := range messages {
		g.mu.Lock()
		for _, v := range g.m {
			for c := range v {
				c <- msg
			}
		}
		g.mu.Unlock()
	}
}
