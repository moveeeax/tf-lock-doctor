package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

// S3Config addresses a lock stored by the Terraform s3 backend. Terraform keeps
// the lock as a single DynamoDB item whose LockID is "<bucket>/<key>-md5" and
// whose Info attribute is the JSON lock record.
type S3Config struct {
	Bucket    string
	Key       string
	Table     string // DynamoDB lock table (backend attr: dynamodb_table)
	Region    string
	Workspace string // defaults to "default"
}

// ddbAPI is the subset of the DynamoDB client tf-lock-doctor uses, extracted so
// tests can substitute a fake.
type ddbAPI interface {
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type s3Backend struct {
	cfg    S3Config
	client ddbAPI
}

// NewS3 builds an S3/DynamoDB backend using ambient AWS credentials (env, shared
// config, or instance role), honouring the region from the backend config.
func NewS3(ctx context.Context, cfg S3Config) (Backend, error) {
	if cfg.Bucket == "" || cfg.Key == "" {
		return nil, errors.New("s3 backend requires bucket and key")
	}
	if cfg.Table == "" {
		return nil, errors.New("s3 backend has no dynamodb_table; state locking is not enabled for this backend")
	}
	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &s3Backend{cfg: cfg, client: dynamodb.NewFromConfig(awsCfg)}, nil
}

// newS3WithClient is used by tests to inject a fake DynamoDB client.
func newS3WithClient(cfg S3Config, client ddbAPI) Backend {
	return &s3Backend{cfg: cfg, client: client}
}

func (b *s3Backend) Name() string { return "s3" }

// lockID reproduces Terraform's DynamoDB LockID for the digest item:
// "<bucket>/<path>" where path includes the workspace prefix for non-default
// workspaces. This is the item that stores the lock Info document.
func (b *s3Backend) lockID() string {
	path := b.cfg.Key
	if ws := b.cfg.Workspace; ws != "" && ws != "default" {
		// Terraform stores non-default workspaces under env:/<ws>/<key>.
		path = "env:/" + ws + "/" + b.cfg.Key
	}
	return b.cfg.Bucket + "/" + path
}

func (b *s3Backend) Read(ctx context.Context) (lock.Info, error) {
	out, err := b.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(b.cfg.Table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: b.lockID()},
		},
	})
	if err != nil {
		return lock.Info{}, fmt.Errorf("dynamodb get lock item: %w", err)
	}
	if len(out.Item) == 0 {
		return lock.Info{}, ErrNoLock
	}
	infoAttr, ok := out.Item["Info"].(*ddbtypes.AttributeValueMemberS)
	if !ok || infoAttr.Value == "" {
		return lock.Info{}, ErrNoLock
	}
	var info lock.Info
	if err := json.Unmarshal([]byte(infoAttr.Value), &info); err != nil {
		return lock.Info{}, fmt.Errorf("decode lock info: %w", err)
	}
	return info, nil
}

func (b *s3Backend) Release(ctx context.Context, id string) error {
	// Re-read under a consistent read and confirm the stored ID matches what the
	// caller intends to release. This closes the window where a different run
	// grabbed the lock between inspect and release.
	current, err := b.Read(ctx)
	if err != nil {
		if errors.Is(err, ErrNoLock) {
			return ErrNoLock
		}
		return err
	}
	if current.ID != id {
		return fmt.Errorf("lock ID changed to %q (expected %q); refusing to unlock a different holder", current.ID, id)
	}
	_, err = b.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(b.cfg.Table),
		Key: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: b.lockID()},
		},
		// Guard against a concurrent re-lock: only delete if the Info still
		// carries this lock ID.
		ConditionExpression:       aws.String("contains(Info, :id)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":id": &ddbtypes.AttributeValueMemberS{Value: id}},
	})
	if err != nil {
		return fmt.Errorf("dynamodb delete lock item: %w", err)
	}
	return nil
}

// FromAttrs builds an S3Config from parsed HCL backend attributes, applying the
// documented Terraform defaults.
func FromAttrs(attrs map[string]string, workspace string) S3Config {
	cfg := S3Config{
		Bucket:    attrs["bucket"],
		Key:       attrs["key"],
		Table:     attrs["dynamodb_table"],
		Region:    attrs["region"],
		Workspace: workspace,
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "default"
	}
	return cfg
}
