package storage

import (
	"strings"
	"testing"
)

func TestApplicationLockKeysMatchOverlappingDestinations(t *testing.T) {
	a := &S3{Config: S3Config{Bucket: "bucket", Prefix: "application", Endpoint: "http://storage"}}
	b := &S3{Config: S3Config{Bucket: "bucket", Prefix: "application/datasets", Endpoint: "http://storage"}}
	x, e := a.ApplicationLockKey("datasets/example/index")
	if e != nil {
		t.Fatal(e)
	}
	y, e := b.ApplicationLockKey("example/index")
	if e != nil {
		t.Fatal(e)
	}
	if x != y || !strings.HasSuffix(x, ":application/datasets/example/index") {
		t.Fatal("overlapping aliases do not lock the same object")
	}
	for _, bad := range []string{"../outside", "/absolute", "a/../outside", "a//b"} {
		if _, e := a.ApplicationKey(bad); e == nil {
			t.Fatal("invalid key accepted", bad)
		}
	}
}
