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

const Timeout = 10 * time.Second
const KeepAlive = 300 // seconds

const statusOnline = "Online"
const statusOffline = "Offline"

type Client interface {
	Publish(quit <-chan struct{}, message, topic string) error
	Subscribe(quit <-chan struct{}, topicFilters ...string) error
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
		for {
			message, topic, err := c.ReadSlices()
			if err != nil {
				log.Printf("Mqtt ReadSlices: %v", err)
				return
			}
			log.Printf("Mqtt <- %v = %v", topic, message)
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

func (m *Mqtt) Publish(quit <-chan struct{}, message, topic string) error {
	if c, err := m.client(); err != nil {
		return err
	} else {
		log.Printf("Mqtt -> %s = %s", topic, message)
		return c.Publish(quit, []byte(message), topic)
	}
}

func (m *Mqtt) Subscribe(quit <-chan struct{}, topicFilters ...string) error {
	if c, err := m.client(); err != nil {
		return err
	} else {
		log.Printf("Mqtt: subcribe to: %v", topicFilters)
		return c.Subscribe(quit, topicFilters...)
	}
}

var DefaultMqtt Mqtt

type Cmd struct {
	Topic, Message string
}

type Expect struct {
	Send *Cmd
	//Recv *Cmd
	// not implemented yet
}

func (e *Expect) Run(ctx context.Context, c Client) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	if e.Send == nil {
		return errors.New("send is a required parameter")
	}
	if err := c.Publish(ctx.Done(), e.Send.Message, e.Send.Topic); err != nil {
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
	defer func() { log.Printf("Mqtt[%s] power on=%v; error=%v", d.GetName(), on, err) }()
	return d.PowerCmd.Run(ctx, d.mqtt())
}
