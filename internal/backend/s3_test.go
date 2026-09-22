package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// fakeDDB is an in-memory stand-in for the DynamoDB client.
type fakeDDB struct {
	item       map[string]ddbtypes.AttributeValue
	deleteErr  error
	deleteHits int
}

func (f *fakeDDB) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if f.item == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: f.item}, nil
}

func (f *fakeDDB) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.deleteHits++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	f.item = nil
	return &dynamodb.DeleteItemOutput{}, nil
}

func lockItem(info string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		"LockID": &ddbtypes.AttributeValueMemberS{Value: "my-bucket/prod/terraform.tfstate"},
		"Info":   &ddbtypes.AttributeValueMemberS{Value: info},
	}
}

const sampleInfo = `{"ID":"abc-123","Operation":"OperationTypeApply","Who":"alice@runner","Created":"2026-01-01T00:00:00Z","Version":"1.7.0"}`

func newTestBackend(f *fakeDDB) Backend {
	return newS3WithClient(S3Config{Bucket: "my-bucket", Key: "prod/terraform.tfstate", Table: "tf-locks", Workspace: "default"}, f)
}

func TestReadLock(t *testing.T) {
	b := newTestBackend(&fakeDDB{item: lockItem(sampleInfo)})
	info, err := b.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info.ID != "abc-123" || info.User() != "alice" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestReadNoLock(t *testing.T) {
	b := newTestBackend(&fakeDDB{item: nil})
	_, err := b.Read(context.Background())
	if !errors.Is(err, ErrNoLock) {
		t.Fatalf("err = %v, want ErrNoLock", err)
	}
}

func TestReleaseRefusesIDMismatch(t *testing.T) {
	f := &fakeDDB{item: lockItem(sampleInfo)}
	b := newTestBackend(f)
	// The lock stored is abc-123; try to release a different, stale ID.
	err := b.Release(context.Background(), "stale-999")
	if err == nil {
		t.Fatal("expected release to refuse an ID mismatch")
	}
	if f.deleteHits != 0 {
		t.Fatalf("DeleteItem was called %d times; must be 0 on mismatch", f.deleteHits)
	}
}

func TestReleaseMatching(t *testing.T) {
	f := &fakeDDB{item: lockItem(sampleInfo)}
	b := newTestBackend(f)
	if err := b.Release(context.Background(), "abc-123"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if f.deleteHits != 1 {
		t.Fatalf("DeleteItem hits = %d, want 1", f.deleteHits)
	}
}

func TestReleaseNoLock(t *testing.T) {
	b := newTestBackend(&fakeDDB{item: nil})
	err := b.Release(context.Background(), "abc-123")
	if !errors.Is(err, ErrNoLock) {
		t.Fatalf("err = %v, want ErrNoLock", err)
	}
}

func TestLockIDNonDefaultWorkspace(t *testing.T) {
	b := &s3Backend{cfg: S3Config{Bucket: "my-bucket", Key: "terraform.tfstate", Table: "t", Workspace: "staging"}}
	got := b.lockID()
	want := "my-bucket/env:/staging/terraform.tfstate"
	if got != want {
		t.Fatalf("lockID() = %q, want %q", got, want)
	}
}
