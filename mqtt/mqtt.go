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
	Publish(ctx context.Context, message, topic string) error
	Subscribe(ctx context.Context, topicFilters ...string) (<-chan *resp, error)
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

	subs map[sub]struct{}
	smu  sync.Mutex
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
	for {
		var r resp
		msg, tc, err := c.ReadSlices()
		r.message, r.topic = string(msg), string(tc)
		if err != nil {
			log.Printf("Mqtt error: %v", err)
			return
		}
		log.Printf("Mqtt <- %v = %v", r.topic, r.message)
		func() {
			m.smu.Lock()
			defer m.smu.Unlock()
			for s := range m.subs {
				select {
				case <-s.ctx.Done():
					close(s.w)
				default:
					s.w <- &r
				}
			}
		}()
	}
}

func (m *Mqtt) client() (*mqtt.Client, error) {
	m.cmu.Lock()
	defer m.cmu.Unlock()
	if m.c != nil {
		return m.c, nil
	}
	c, err := mqtt.VolatileSession(m.clientID(), m.config())
	if err != nil {
		return nil, err
	}
	m.c = c
	m.smu.Lock()
	defer m.smu.Unlock()
	if m.subs == nil {
		m.subs = make(map[sub]struct{})
	}
	go m.readLoop(m.c)
	return m.c, m.c.PublishRetained(nil, []byte(statusOnline), m.statusTopic())
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

func (m *Mqtt) Publish(ctx context.Context, message, topic string) error {
	if c, err := m.client(); err != nil {
		return err
	} else {
		log.Printf("Mqtt -> %s = %s", topic, message)
		return c.Publish(ctx.Done(), []byte(message), topic)
	}
}

func (m *Mqtt) Subscribe(ctx context.Context, topicFilters ...string) (<-chan *resp, error) {
	c, err := m.client()
	if err != nil {
		return nil, err
	}
	log.Printf("Mqtt: subcribe: %v", topicFilters)
	if err = c.Subscribe(ctx.Done(), topicFilters...); err != nil {
		return nil, err
	}
	w := make(chan *resp)
	// w closed by goroutine below
	s := sub{
		ctx,
		w,
	}
	m.smu.Lock()
	defer m.smu.Unlock()
	m.subs[s] = struct{}{}
	go func() {
		defer close(w)
		<-ctx.Done()
		m.smu.Lock()
		defer m.smu.Unlock()
		delete(m.subs, s)
	}()
	return w, nil
}

var DefaultMqtt Mqtt

type sub struct {
	ctx context.Context
	w   chan *resp
}

type resp struct {
	message, topic string
}

type Cmd struct {
	Topic, Message string
}

type Expect struct {
	Send *Cmd
	Recv *Cmd
}

func (e *Expect) Run(ctx context.Context, c Client) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	if e.Send == nil {
		return errors.New("send is a required parameter")
	}
	var rchan <-chan *resp
	if e.Recv != nil && e.Recv.Topic != "" {
		var err error
		if rchan, err = c.Subscribe(ctx, e.Recv.Topic); err != nil {
			return err
		}
	}
	if err := c.Publish(ctx, e.Send.Message, e.Send.Topic); err != nil {
		return err
	}
	if e.Recv != nil && e.Recv.Topic != "" {
		ret := errors.New("expected mqtt messsage not received")
		for r := range rchan {
			// Important: consume the entire channel, even if we don't care,
			// so we don't deadlock the sender.
			if r.topic == e.Recv.Topic {
				if r.message != "" && r.message != e.Recv.Message {
					// Only check message if specified. Else, any message means success.
					continue
				}
				ret = nil
				cancel() // ask sender to unsubscribe and close rchan
			}
		}
		return ret
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
