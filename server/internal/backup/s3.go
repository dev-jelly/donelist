package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// S3Storage handles S3-compatible storage operations for backups
type S3Storage struct {
	config *Config
	client *s3.Client
	logger *zap.Logger
}

// NewS3Storage creates a new S3 storage handler
func NewS3Storage(cfg *Config, logger *zap.Logger) (*S3Storage, error) {
	// Create AWS config with custom endpoint for S3-compatible storage
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if cfg.S3Endpoint != "" {
			return aws.Endpoint{
				URL:               cfg.S3Endpoint,
				HostnameImmutable: true,
				SigningRegion:     cfg.S3Region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.S3Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID,
			cfg.S3SecretAccessKey,
			"",
		)),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // Required for MinIO and some S3-compatible storage
	})

	return &S3Storage{
		config: cfg,
		client: client,
		logger: logger,
	}, nil
}

// UploadBackup uploads a backup file to S3
func (s *S3Storage) UploadBackup(ctx context.Context, localPath string) error {
	s.logger.Info("Uploading backup to S3",
		zap.String("local_path", localPath),
		zap.String("bucket", s.config.S3Bucket),
	)

	// Open local file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info for metadata
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Generate S3 key with date-based prefix for organization
	filename := filepath.Base(localPath)
	timestamp := time.Now()
	s3Key := fmt.Sprintf("backups/%d/%02d/%02d/%s",
		timestamp.Year(), timestamp.Month(), timestamp.Day(), filename)

	// Prepare metadata
	metadata := map[string]string{
		"backup-type":   "postgres",
		"backup-date":   timestamp.Format(time.RFC3339),
		"original-name": filename,
		"file-size":     fmt.Sprintf("%d", fileInfo.Size()),
	}

	// Upload to S3
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.config.S3Bucket),
		Key:           aws.String(s3Key),
		Body:          file,
		ContentLength: aws.Int64(fileInfo.Size()),
		Metadata:      metadata,
		StorageClass:  types.StorageClassStandardIa, // Infrequent Access for cost savings
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	s.logger.Info("Backup uploaded successfully",
		zap.String("s3_key", s3Key),
		zap.Int64("size", fileInfo.Size()),
	)

	return nil
}

// DownloadBackup downloads a backup file from S3
func (s *S3Storage) DownloadBackup(ctx context.Context, s3Key, localPath string) error {
	s.logger.Info("Downloading backup from S3",
		zap.String("s3_key", s3Key),
		zap.String("local_path", localPath),
	)

	// Create local directory if it doesn't exist
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Get object from S3
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.S3Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	// Create local file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	written, err := io.Copy(file, result.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	s.logger.Info("Backup downloaded successfully",
		zap.String("s3_key", s3Key),
		zap.Int64("size", written),
	)

	return nil
}

// ListBackups lists all backups in S3
func (s *S3Storage) ListBackups(ctx context.Context, prefix string) ([]types.Object, error) {
	s.logger.Info("Listing backups from S3",
		zap.String("prefix", prefix),
	)

	var allObjects []types.Object
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.config.S3Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}

		result, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		allObjects = append(allObjects, result.Contents...)

		if !aws.ToBool(result.IsTruncated) {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return allObjects, nil
}

// DeleteBackup deletes a backup file from S3
func (s *S3Storage) DeleteBackup(ctx context.Context, s3Key string) error {
	s.logger.Info("Deleting backup from S3", zap.String("s3_key", s3Key))

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.config.S3Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	s.logger.Info("Backup deleted successfully", zap.String("s3_key", s3Key))
	return nil
}

// DeleteMultipleBackups deletes multiple backup files from S3
func (s *S3Storage) DeleteMultipleBackups(ctx context.Context, s3Keys []string) error {
	if len(s3Keys) == 0 {
		return nil
	}

	s.logger.Info("Deleting multiple backups from S3", zap.Int("count", len(s3Keys)))

	// S3 allows deleting up to 1000 objects at once
	const batchSize = 1000

	for i := 0; i < len(s3Keys); i += batchSize {
		end := i + batchSize
		if end > len(s3Keys) {
			end = len(s3Keys)
		}

		batch := s3Keys[i:end]
		objects := make([]types.ObjectIdentifier, len(batch))
		for j, key := range batch {
			objects[j] = types.ObjectIdentifier{
				Key: aws.String(key),
			}
		}

		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.config.S3Bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects (batch %d-%d): %w", i, end, err)
		}
	}

	s.logger.Info("Multiple backups deleted successfully", zap.Int("count", len(s3Keys)))
	return nil
}

// GetBackupsByAge groups backups by age categories
func (s *S3Storage) GetBackupsByAge(ctx context.Context) (map[string][]types.Object, error) {
	allBackups, err := s.ListBackups(ctx, "backups/")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	categories := map[string][]types.Object{
		"daily":   {},
		"weekly":  {},
		"monthly": {},
		"old":     {},
	}

	for _, obj := range allBackups {
		if obj.LastModified == nil {
			continue
		}

		age := now.Sub(*obj.LastModified)
		switch {
		case age <= 7*24*time.Hour:
			categories["daily"] = append(categories["daily"], obj)
		case age <= 28*24*time.Hour:
			categories["weekly"] = append(categories["weekly"], obj)
		case age <= 365*24*time.Hour:
			categories["monthly"] = append(categories["monthly"], obj)
		default:
			categories["old"] = append(categories["old"], obj)
		}
	}

	// Sort each category by modification time (newest first)
	for category := range categories {
		sort.Slice(categories[category], func(i, j int) bool {
			return categories[category][i].LastModified.After(*categories[category][j].LastModified)
		})
	}

	return categories, nil
}

// GetBackupMetadata retrieves metadata for a backup
func (s *S3Storage) GetBackupMetadata(ctx context.Context, s3Key string) (map[string]string, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.config.S3Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	metadata := make(map[string]string)
	for key, value := range result.Metadata {
		metadata[key] = value
	}

	// Add standard metadata
	if result.ContentLength != nil {
		metadata["content-length"] = fmt.Sprintf("%d", *result.ContentLength)
	}
	if result.LastModified != nil {
		metadata["last-modified"] = result.LastModified.Format(time.RFC3339)
	}

	return metadata, nil
}

// EnsureBucketExists ensures the S3 bucket exists, creates it if not
func (s *S3Storage) EnsureBucketExists(ctx context.Context) error {
	s.logger.Info("Checking if S3 bucket exists", zap.String("bucket", s.config.S3Bucket))

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.config.S3Bucket),
	})
	if err == nil {
		s.logger.Info("S3 bucket exists", zap.String("bucket", s.config.S3Bucket))
		return nil
	}

	// Bucket doesn't exist, create it
	s.logger.Info("Creating S3 bucket", zap.String("bucket", s.config.S3Bucket))

	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.config.S3Bucket),
	})
	if err != nil {
		// Check if error is because bucket already exists
		if strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") ||
			strings.Contains(err.Error(), "BucketAlreadyExists") {
			s.logger.Info("S3 bucket already exists", zap.String("bucket", s.config.S3Bucket))
			return nil
		}
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	s.logger.Info("S3 bucket created successfully", zap.String("bucket", s.config.S3Bucket))
	return nil
}
