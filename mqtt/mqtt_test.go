package mqtt

import (
	"errors"
	"testing"
)

type T int

// Tests that start() and finish() are called in the right sequence.
func TestListenGroupStartFinishCounts(t *testing.T) {
	var lg listenGroup[T]

	startCount := 0
	finishCount := 0
	start := func() error {
		startCount += 1
		return nil
	}
	finish := func() {
		finishCount += 1
	}
	_, done1, err := lg.subscribe("rabbits",
		start,
		func() { t.Error("wrong finish") })
	if err != nil {
		t.Error(err)
	}
	_, done2, err := lg.subscribe("rabbits",
		func() error { t.Error("wrong start"); return nil },
		finish)
	if err != nil {
		t.Error(err)
	}

	if got, want := startCount, 1; got != want {
		t.Errorf("startCount: got %d, want %d", got, want)
	}
	if got, want := finishCount, 0; got != want {
		t.Errorf("finishCount: got %d, want %d", got, want)
	}

	done1()

	if got, want := startCount, 1; got != want {
		t.Errorf("startCount: got %d, want %d", got, want)
	}
	if got, want := finishCount, 0; got != want {
		t.Errorf("finishCount: got %d, want %d", got, want)
	}

	done2()

	if got, want := startCount, 1; got != want {
		t.Errorf("startCount: got %d, want %d", got, want)
	}
	if got, want := finishCount, 1; got != want {
		t.Errorf("finishCount: got %d, want %d", got, want)
	}
}

// Tests that messages are delivered.
func TestListenGroupMessages(t *testing.T) {
	var lg listenGroup[T]

	start := func() error { return nil }
	finished := make(chan struct{})
	finish := func() { close(finished) }
	m1, done1, err := lg.subscribe("rabbits", start, finish)
	if err != nil {
		t.Error(err)
	}
	m2, done2, err := lg.subscribe("rabbits", start, finish)
	if err != nil {
		t.Error(err)
	}

	const n T = 20

	pub := make(chan T, n*2)
	go func() {
		for i := T(0); i < n; i++ {
			pub <- 1
		}
		close(pub)
	}()

	counter := func(i *T, c <-chan T, done func()) {
		for *i < n {
			*i += <-c
		}
		done()
	}

	var m1count, m2count T
	go counter(&m1count, m1, done1)
	go counter(&m2count, m2, done2)

	lg.publish(pub)
	<-finished

	if got1, got2, want := m1count, m2count, n; got1 != want || got2 != want {
		t.Errorf("got m1count=%d, m2count=%d, want n=%d", m1count, m2count, n)
	}
}

// Tests that unsubscribes are cleaned up.
func TestListenGroupUnsubscribe(t *testing.T) {
	var lg listenGroup[T]

	start := func() error { return nil }
	finish := func() {}
	m1, done1, err := lg.subscribe("rabbits", start, finish)
	if err != nil {
		t.Error(err)
	}
	m2, done2, err := lg.subscribe("rabbits", start, finish)
	if err != nil {
		t.Error(err)
	}

	lg.mu.Lock()
	if _, ok := lg.m["rabbits"]; !ok {
		t.Fatal(`lg.m["rabbits"] missing`)
	}
	lg.mu.Unlock()

	pub := make(chan T) // unbuffered
	defer close(pub)
	go lg.publish(pub)
	go func() { pub <- 1 }() // should block, with listeners
	for i := 0; i < 2; i++ {
		select {
		case <-m1:
		case <-m2:
		}
	}
	done1()
	done2()

	pub <- 1 // should not block, with no listeners

	lg.mu.Lock()
	if _, ok := lg.m["rabbits"]; ok {
		t.Error(`lg.m["rabbits"] should not be present`)
	}
	lg.mu.Unlock()
}

// Tests that an error in start() doesn't cause side-effects.
func TestListenGroupStartErr(t *testing.T) {
	var lg listenGroup[T]

	errTest := errors.New("a test error")
	startErr := func() error {
		return errTest
	}

	m, done, err := lg.subscribe("rabbits", startErr, nil)
	if !errors.Is(err, errTest) {
		t.Errorf("lg.subscribe: err: got %v, want %v", err, errTest)
	}
	if m != nil {
		t.Error("lg.subscribe: msgsR: got chan, want nil")
	}
	if done != nil {
		t.Error("lg.subscribe: done: got func, want nil")
	}

	if _, ok := lg.m["rabbits"]; ok {
		t.Error(`lg.m["rabbits"] should not be present`)
	}
}
