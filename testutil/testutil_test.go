package testutil

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type fakeTB struct {
	testing.TB
	failures []string
}

func (f *fakeTB) Error(args ...any) {
	f.failures = append(f.failures, fmt.Sprint(args...))
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.failures = append(f.failures, fmt.Sprintf(format, args...))
}

func TestAssertCachedValueStopsOnTypeMismatch(t *testing.T) {
	ftb := &fakeTB{}

	// The nil cache must never be touched: on a type mismatch the helper
	// has to report and return before comparing cached values.
	AssertCachedValue[*wrapperspb.StringValue](context.Background(), ftb, nil, "key", &wrapperspb.Int64Value{})

	if len(ftb.failures) != 1 {
		t.Fatalf("expected exactly one reported failure, got %d: %v", len(ftb.failures), ftb.failures)
	}
}

func TestAssertCachedValueRejectsInterfaceTypeParam(t *testing.T) {
	ftb := &fakeTB{}

	AssertCachedValue[protoreflect.ProtoMessage](context.Background(), ftb, nil, "key", &wrapperspb.StringValue{})

	if len(ftb.failures) != 1 {
		t.Fatalf("expected exactly one reported failure, got %d: %v", len(ftb.failures), ftb.failures)
	}
}
