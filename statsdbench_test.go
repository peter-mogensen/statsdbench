package statsdbench

import (
	"net"
	"strconv"
	"testing"
	"time"

	cactus "github.com/cactus/go-statsd-client/statsd"
	"github.com/peterbourgon/g2s"
	quipo "github.com/quipo/statsd"
	ac "gopkg.in/alexcesaro/statsd.v2"
	gone "github.com/One-com/gone/metric"
	gones "github.com/One-com/gone/metric/sink/statsd"
)

const (
	addr        = "127.0.0.1:0"
	prefix      = "prefix."
	prefixNoDot = "prefix"
	counterKey  = "foo.bar.counter"
	gaugeKey    = "foo.bar.gauge"
	gaugeValue  = 42
	timingKey   = "foo.bar.timing"
	timingValue = 153 * time.Millisecond
	flushPeriod = 100 * time.Millisecond
)

type logger struct{}

func (logger) Println(v ...interface{}) {}

func BenchmarkGoneS(b *testing.B) {
	s := newServer()

	sink, err := gones.New(
		gones.Peer(s.Addr()),
		gones.Prefix(prefixNoDot),
		gones.Buffer(1432))
	if err != nil {
		b.Fatal(err)
	}

	flushPeriod := gone.FlushInterval(flushPeriod)
	c := gone.NewClient(sink, flushPeriod)

	gauge   := c.RegisterGauge(gaugeKey)
	timer   := c.RegisterTimer(timingKey)
	counter := c.RegisterCounter(counterKey)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		counter.Inc(1)
		gauge.Set(gaugeValue)
		timer.Sample(timingValue)
	}

	c.Stop()
	c.Flush()
	s.Close()
}

func BenchmarkGoneP(b *testing.B) {
	s := newServer()

	sink, err := gones.New(
		gones.Peer(s.Addr()),
		gones.Prefix(prefixNoDot),
		gones.Buffer(1432))
	if err != nil {
		b.Fatal(err)
	}
	flushPeriod := gone.FlushInterval(flushPeriod)
	c := gone.NewClient(sink, flushPeriod)

	gauge   := c.RegisterGauge(gaugeKey)
	timer   := c.RegisterTimer(timingKey)
	counter := c.RegisterCounter(counterKey)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Inc(1)
			gauge.Set(gaugeValue)
			timer.Sample(timingValue)
		}
	})

	c.Stop()
	c.Flush()
	s.Close()
}

func BenchmarkAlexcesaroS(b *testing.B) {
	s := newServer()
	c, err := ac.New(
		ac.Address(s.Addr()),
		ac.Prefix(prefixNoDot),
		ac.FlushPeriod(flushPeriod),
	)
	if err != nil {
		b.Fatal(err)
	}

	t := int(timingValue)/1000000

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Increment(counterKey)
		c.Gauge(gaugeKey, gaugeValue)
		c.Timing(timingKey, t)
	}
	c.Close()
	s.Close()
}

func BenchmarkAlexcesaroP(b *testing.B) {
	s := newServer()
	c, err := ac.New(
		ac.Address(s.Addr()),
		ac.Prefix(prefixNoDot),
		ac.FlushPeriod(flushPeriod),
	)
	if err != nil {
		b.Fatal(err)
	}

	t := int(timingValue)/1000000

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Increment(counterKey)
			c.Gauge(gaugeKey, gaugeValue)
			c.Timing(timingKey, t)
		}
	})
	c.Close()
	s.Close()
}

func BenchmarkCactus(b *testing.B) {
	s := newServer()
	c, err := cactus.NewBufferedClient(s.Addr(), prefix, flushPeriod, 1432)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Inc(counterKey, 1, 1)
		c.Gauge(gaugeKey, gaugeValue, 1)
		c.Timing(timingKey, int64(timingValue), 1)
	}
	c.Close()
	s.Close()
}

func BenchmarkG2s(b *testing.B) {
	s := newServer()
	c, err := g2s.Dial("udp", s.Addr())
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Counter(1, counterKey, 1)
		c.Gauge(1, gaugeKey, strconv.Itoa(gaugeValue))
		c.Timing(1, timingKey, timingValue)
	}
	s.Close()
}

func BenchmarkQuipo(b *testing.B) {
	s := newServer()
	c := quipo.NewStatsdBuffer(flushPeriod, quipo.NewStatsdClient(s.Addr(), prefix))
	c.Logger = logger{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Incr(counterKey, 1)
		c.Gauge(gaugeKey, gaugeValue)
		c.Timing(timingKey, int64(timingValue))
	}
	c.Close()
	s.Close()
}

type server struct {
	conn   *net.UDPConn
	closed chan bool
}

func newServer() *server {
	addr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	s := &server{conn: conn, closed: make(chan bool)}
	go func() {
		buf := make([]byte, 1500)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				s.closed <- true
				return
			}
		}
	}()
	return s
}

func (s *server) Addr() string {
	return s.conn.LocalAddr().String()
}

func (s *server) Close() {
	s.conn.Close()
	<-s.closed
	return
}
