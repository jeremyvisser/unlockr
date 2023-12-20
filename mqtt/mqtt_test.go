package mqtt

import (
	"testing"
)

type T int

// Tests that messages are delivered.
func TestListenGroupMessages(t *testing.T) {
	var lg listenGroup[T]

	done1, done2 := make(chan struct{}), make(chan struct{})
	m1, m2 := lg.subscribe(done1), lg.subscribe(done2)

	const n T = 20

	// Publish messages to channel:
	pub := make(chan T, n*2)
	go func() {
		for i := T(0); i < n; i++ {
			pub <- 1
		}
		close(pub)
	}()

	// Count messages received from channel:
	counter := func(i *T, c <-chan T, done chan struct{}) {
		for *i < n {
			*i += <-c
		}
		close(done)
	}

	var m1count, m2count T
	go counter(&m1count, m1, done1)
	go counter(&m2count, m2, done2)

	lg.publish(pub)
	<-m1
	<-m2

	if got1, got2, want := m1count, m2count, n; got1 != want || got2 != want {
		t.Errorf("got m1count=%d, m2count=%d, want n=%d", m1count, m2count, n)
	}
}

// Tests that unsubscribes are cleaned up.
func TestListenGroupUnsubscribe(t *testing.T) {
	var lg listenGroup[T]

	done1, done2 := make(chan struct{}), make(chan struct{})
	m1, m2 := lg.subscribe(done1), lg.subscribe(done2)

	lg.mu.Lock()
	if got, want := len(lg.m), 2; got != want {
		t.Fatalf("len(lg.m): got %d, want %d", got, want)
	}
	lg.mu.Unlock()

	close(done1)
	close(done2)
	<-m1
	<-m2

	pub := make(chan T)
	go func() {
		pub <- 1 // should not block, because we have no listeners
		close(pub)
	}()
	lg.publish(pub)

	lg.mu.Lock()
	if got, want := len(lg.m), 0; got != want {
		t.Fatalf("len(lg.m): got %d, want %d\n  m1: %#v\n  m2: %#v\n  lg.m: %#v", got, want, m1, m2, lg.m)
	}
	lg.mu.Unlock()
}
