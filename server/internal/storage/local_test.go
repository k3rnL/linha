package storage

import (
	"bytes"
	"context"
	"io"
	"linha/server/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentImmutableContainedResults(t *testing.T) {
	root := t.TempDir()
	local, e := NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	a := domain.Allocation{ID: "o", Name: "result.json", Key: "context/job/attempt/result.json", Kind: "json", ContentType: "application/json", Policy: domain.ResultPolicy{Type: "local", Root: root, MaxBytes: 1024}}
	f, e := local.Put(context.Background(), a, bytes.NewBufferString(`{"n":42}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = local.Put(context.Background(), a, bytes.NewBufferString(`{"n":99}`)); e == nil {
		t.Fatal("replaced immutable result")
	}
	local.Close()
	local, e = NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer local.Close()
	if e = local.Verify(context.Background(), a, f); e != nil {
		t.Fatal(e)
	}
	reader, e := local.Open(context.Background(), a)
	if e != nil {
		t.Fatal(e)
	}
	content, e := io.ReadAll(reader)
	reader.Close()
	if e != nil || string(content) != `{"n":42}` {
		t.Fatal("result lost")
	}
	outside := t.TempDir()
	if e = os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	a.Key = "escape/stolen"
	if _, e = local.Put(context.Background(), a, bytes.NewBufferString("no")); e == nil {
		t.Fatal("followed escaping symlink")
	}
	a.Key = "../stolen"
	if _, e = local.Put(context.Background(), a, bytes.NewBufferString("no")); e == nil {
		t.Fatal("accepted traversal")
	}
}
func TestLimitNeverPublishesPartialOutput(t *testing.T) {
	root := t.TempDir()
	s, e := NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a := domain.Allocation{ID: "o", Name: "a", Key: "job/attempt/a", Kind: "file", ContentType: "text/plain", Policy: domain.ResultPolicy{Type: "local", Root: root, MaxBytes: 2}}
	if _, e = s.Put(context.Background(), a, bytes.NewBufferString("too big")); e == nil {
		t.Fatal("accepted over limit")
	}
	if _, e = s.Open(context.Background(), a); e == nil {
		t.Fatal("published partial file")
	}
}
