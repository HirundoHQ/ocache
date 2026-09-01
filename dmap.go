package ocache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/hirundohq/logging"
	"github.com/olric-data/olric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type paramOptions struct {
	hash string
}

type Key string

type KeyOption func(*paramOptions)

type DMap struct {
	dm  olric.DMap
	log logging.Logger
}

func (oc *Client) NewDMap(name string) (*DMap, error) {
	dm, err := oc.c.NewDMap(name)
	if err != nil {
		return nil, fmt.Errorf("olric.NewDMap returned an error: %w", err)
	}

	return &DMap{
		dm:  dm,
		log: oc.log,
	}, nil
}

// CreateKey creates a cache key using the DMap name and the provided name and options.
func (o *DMap) CreateKey(name string, opts ...KeyOption) Key {
	if opts == nil {
		return Key(strings.Join([]string{o.dm.Name(), name}, "_"))
	}

	po := &paramOptions{}
	for _, opt := range opts {
		opt(po)
	}

	return Key(strings.Join([]string{o.dm.Name(), name, po.hash}, "_"))
}

// Put will put a value in the cache with a given key and a given timeout
func (o *DMap) PutEx(ctx context.Context, key Key, value any, timeout time.Duration) {
	if value == nil {
		return
	}

	ctx = context.WithoutCancel(ctx)
	if err := o.dm.Put(ctx, string(key), value, olric.EX(timeout)); err != nil {
		o.log.Debug("olric.PutEx: failed to put key", "key", key, "timeout", timeout, "error", err)
		o.log.Error("olric.PutEx: failed to put key", "error", err)
	}
}

// Put will put a value in the cache with a given key
func (o *DMap) Delete(ctx context.Context, keys ...Key) {
	strKeys := make([]string, len(keys))
	for i, key := range keys {
		strKeys[i] = string(key)
	}
	_, err := o.dm.Delete(ctx, strKeys...)
	if err != nil && err.Error() != olric.ErrKeyNotFound.Error() {
		o.log.Debug("olric.Delete: failed to delete keys", "keys", keys, "error", err)
		o.log.Error("olric.Delete: failed to delete keys", "error", err)
	}
}

// Get will get a value from the cache with a given key
func (o *DMap) Get(ctx context.Context, key Key) *olric.GetResponse {
	val, err := o.dm.Get(ctx, string(key))
	if err != nil && err.Error() != olric.ErrKeyNotFound.Error() {
		o.log.Debug("olric.Get: failed to get key", "key", key, "error", err)
		o.log.Error("olric.Get: failed to get key", "error", err)
	}

	if val != nil {
		o.log.Debug("cache", "key", key)
	}

	return val
}

// Flush will invalidate the cache (delete all keys).
// This is useful when you want to force a refresh of the cache for the given DMap name.
func (o *DMap) Flush(ctx context.Context) {
	if err := o.dm.Destroy(ctx); err != nil {
		o.log.Error("olric.Flush: failed to invalidate (flush) cache", "error", err)
	}
}

// FlushKeys will invalidate the cache for all keys that match the given pattern.
func (o *DMap) FlushKeys(ctx context.Context, pattern string) {
	i, err := o.dm.Scan(ctx, olric.Match(pattern))
	if err != nil {
		o.log.Error("olric.FlushKeys: failed to scan keys", "error", err)
		return
	}
	defer i.Close()

	for i.Next() {
		_, err := o.dm.Delete(ctx, i.Key())
		if err != nil {
			o.log.Debug("olric.Delete: failed to delete key", "key", i.Key(), "error", err)
			o.log.Error("olric.Delete: failed to delete key", "error", err)
		}
	}
}

// WithParams is a KeyOption that allows you to pass request parameters to the key builder.
func WithParams(requestParams string) KeyOption {
	return func(po *paramOptions) {
		hash := sha256.Sum256([]byte(requestParams))
		po.hash = hex.EncodeToString(hash[:])
	}
}

// WithProtoMsg is a KeyOption that allows you to pass a proto message to the key builder.
func WithProtoMsg(req proto.Message) KeyOption {
	return func(po *paramOptions) {
		dataBytes, err := proto.Marshal(req)
		if err != nil {
			dataBytes = []byte(protoimpl.X.MessageStringOf(req))
		}
		hash := sha256.Sum256(dataBytes)
		po.hash = hex.EncodeToString(hash[:])
	}
}

// GetProto retrieves and unmarshals a proto message from cache.
// It handles the marshaller wrapper internally, so you only need to pass the proto message.
// The message is updated in-place by reference.
//
// Usage:
//
//	msg := &pb.MyMessage{}
//	if found := cache.GetProto(ctx, key, msg); found {
//	    // use msg - it's now populated with cached data
//	}
func (o *DMap) GetProto(ctx context.Context, key Key, protoMsg protoreflect.ProtoMessage) bool {
	cachedValue := o.Get(ctx, key)
	if cachedValue == nil {
		return false
	}

	// Create wrapper internally - user doesn't need to know about it
	wrapper := newProtoMarshaller(protoMsg)
	if err := cachedValue.Scan(wrapper); err != nil {
		o.log.Debug("olric.GetProto: failed to scan proto message", "key", key, "error", err)
		o.log.Error("olric.GetProto: failed to unmarshal cached value", "error", err)
		return false
	}

	// The protoMsg is already updated by reference through the wrapper
	return true
}

// PutProto stores a proto message in cache with the given key and timeout.
// It handles the marshaller wrapper internally.
//
// Usage:
//
//	msg := &pb.MyMessage{...}
//	cache.PutProto(ctx, key, msg, 1*time.Hour)
func (o *DMap) PutProto(ctx context.Context, key Key, protoMsg protoreflect.ProtoMessage, timeout time.Duration) {
	if protoMsg == nil {
		return
	}

	wrapper := newProtoMarshaller(protoMsg)
	o.PutEx(ctx, key, wrapper, timeout)
}
