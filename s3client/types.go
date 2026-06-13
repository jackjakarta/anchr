package s3client

import "time"

type S3Item struct {
	Key          string
	Name         string
	IsDir        bool
	Size         int64
	LastModified time.Time
	ETag         string // entity tag (unquoted), empty for directories
	StorageClass string // e.g. "STANDARD", empty for directories
}

type ListResult struct {
	Items  []S3Item
	Prefix string
	Bucket string
}
