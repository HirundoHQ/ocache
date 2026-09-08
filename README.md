# ocache

## Olric Client with Proto Message Support

This package provides a clean abstraction over Olric distributed cache with native support for Protocol Buffer messages. Proto marshalling is handled internally, keeping the API simple and idiomatic.

## Installation

```sh
go get github.com/hirundohq/ocache
```

## API

### GetProto

Retrieves and unmarshals a proto message from cache.

```go
func (o *DMap) GetProto(ctx context.Context, key Key, protoMsg protoreflect.ProtoMessage) bool
```

- Returns `bool` indicating if the key was found
- Updates message in-place by reference
- Handles errors internally with logging

### PutProto

Stores a proto message in cache with expiration.

```go
func (o *DMap) PutProto(ctx context.Context, key Key, protoMsg protoreflect.ProtoMessage, timeout time.Duration)
```

- Wraps message automatically for storage
- Accepts any proto message
- Sets expiration time

## Usage Examples

### Basic Read/Write

```go
// Write to cache
req := &pb.Request{...}
cache.PutProto(ctx, key, req, 1*time.Hour)

// Read from cache
response := &pb.Response{}
if !cache.GetProto(ctx, key, response) {
    return nil, status.Errorf(codes.NotFound, "not found")
}
```

### Cache Middleware Pattern

```go
func (m *middleware) GetOffers(ctx context.Context, req *pb.Request) ([]*pb.Offer, error) {
    cacheKey := m.cache.CreateKey("offers", ocache.WithProtoMsg(req))
    
    // Try cache first
    wrapper := &pb.OffersWrapper{}
    if m.cache.GetProto(ctx, cacheKey, wrapper) {
        return wrapper.GetOffers(), nil
    }
    
    // Cache miss - fetch from source
    offers, err := m.next.GetOffers(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // Store in cache
    wrapper.Offers = offers
    m.cache.PutProto(ctx, cacheKey, wrapper, utils.CacheTime)
    
    return offers, nil
}
```

## Key Generation

### WithProtoMsg

Creates deterministic cache keys from proto message content:

```go
key := cache.CreateKey("prefix", ocache.WithProtoMsg(request))
```

Identical requests produce identical cache keys, maximizing cache hits.

### WithParams

Creates cache keys from string parameters:

```go
key := cache.CreateKey("prefix", ocache.WithParams(userID))
```

## Miss vs. failure

`Get` returns `nil` both when the key is absent and when the cluster could not
answer, which suits a read-through cache. When the caller must tell the two
apart — a shared credential, for example — use `Lookup`:

```go
val, found, err := dm.Lookup(ctx, key)
switch {
case err != nil:   // Olric unreachable: fall back to local state
case !found:       // key absent or expired: produce the value
default:           // hit
}
```

## Locks

`WithLock` runs a function under a cluster-wide lock, so replicas doing the
same expensive work (a provider login, say) do it once:

```go
err := dm.WithLock(ctx, dm.CreateKey("token_lock"), 30*time.Second, 10*time.Second, func(ctx context.Context) error {
    // at most one caller across the cluster is here at a time
    return nil
})
if errors.Is(err, ocache.ErrLockNotAcquired) {
    // someone else held it for the whole wait: degrade, do not fail
}
```

The lock expires after `ttl` even if its holder dies, so pick a `ttl` above
the function's worst case. Olric locks are advisory — a key with a TTL,
polled every 10 ms — so use them to avoid duplicate work, never to protect an
invariant.

## Expiration Strategies

```go
// Fixed duration
cache.PutProto(ctx, key, msg, 1*time.Hour)

// Until specific time
cache.PutProto(ctx, key, msg, utils.Till8AMSerbia())

// Dynamic duration
cache.PutProto(ctx, key, msg, utils.CacheTime)
```

## Implementation Details

Proto messages don't natively implement the `encoding.BinaryMarshaler` and `encoding.BinaryUnmarshaler` interfaces required by Olric. This package includes an internal marshaller that wraps proto messages transparently, so users work directly with proto messages without manual wrapper management.
