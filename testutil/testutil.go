package testutil

import (
	"context"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-test/deep"
	"github.com/hirundohq/ocache"
	"github.com/olric-data/olric"
	"github.com/olric-data/olric/config"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type OlricServer struct {
	db       *olric.Olric
	endpoint string
}

func RunServer() (*OlricServer, error) {
	port, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	c := config.New("local")
	c.LogLevel = "ERROR"
	c.LogVerbosity = 1
	c.MemberlistConfig.BindPort = 0
	c.MemberlistConfig.BindAddr = "127.0.0.1"
	c.BindAddr = "127.0.0.1"
	c.BindPort = port

	err = c.Sanitize()
	if err != nil {
		return nil, fmt.Errorf("failed to sanitize config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.Started = func() {
		cancel()
	}

	db, err := olric.New(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create Olric: %w", err)
	}

	go func() {
		if err := db.Start(); err != nil {
			panic(fmt.Sprintf("failed to run Olric: %v", err))
		}
	}()

	select {
	case <-time.After(time.Second):
		panic("olric cannot be started in one second")
	case <-ctx.Done():
		log.Printf("Olric is ready to accept connections on %s:%d", c.BindAddr, c.BindPort)
		// everything is fine
	}

	return &OlricServer{
		db:       db,
		endpoint: fmt.Sprintf("%s:%d", c.BindAddr, c.BindPort),
	}, nil
}

func (s *OlricServer) Close() {
	if s.db != nil {
		if err := s.db.Shutdown(context.Background()); err != nil {
			panic(fmt.Sprintf("failed to shutdown Olric: %v", err))
		}
		log.Print("Olric is shut down")
	}
}

func (s *OlricServer) Endpoint() string {
	return s.endpoint
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func AssertCachedValue[T protoreflect.ProtoMessage](
	ctx context.Context,
	t testing.TB,
	cache *ocache.DMap,
	cacheKey ocache.Key,
	expectedResponse protoreflect.ProtoMessage,
) {
	var msg T // Constrained to proto.Message

	// Peek the type inside T (as T= *SomeProtoMsgType)
	rt := reflect.TypeOf(msg)
	if rt == nil {
		t.Errorf("type parameter T must be a concrete proto message type, not an interface")
		return
	}
	msgType := rt.Elem()

	// Make a new one, and throw it back into T
	msg = reflect.New(msgType).Interface().(T)

	msgExpectedType := reflect.TypeOf(expectedResponse).Elem()

	if msgType != msgExpectedType {
		t.Errorf("msg type not as expected, got: %s, expected: %s", msgType, msgExpectedType)
		return
	}

	if !cache.GetProto(ctx, cacheKey, msg) {
		t.Errorf("cached value not found or failed to unmarshal")
		return
	}

	if !proto.Equal(msg, expectedResponse) {
		t.Error("unexpected cached response")
		if diff := deep.Equal(msg, expectedResponse); diff != nil {
			t.Errorf("diff:\n%v\n", strings.Join(diff, "\n"))
		}
	}
}
