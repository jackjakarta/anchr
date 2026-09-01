package ui

import "github.com/jackjakarta/anchr/s3client"

type BucketSelectedMsg struct {
	Index int
}

type ObjectsLoadedMsg struct {
	Result *s3client.ListResult
	Err    error
}

type NavigateMsg struct {
	Prefix string
}

// DownloadProgressMsg is one sample of an in-flight download's counters, taken
// on a timer by pollProgress. Total is 0 while the object size is still unknown.
type DownloadProgressMsg struct {
	Written int64
	Total   int64
}

// FileDownloadedMsg is emitted after a download completes, fails, or is aborted
// with ctrl+x (Cancelled, which the status line reports instead of the error).
type FileDownloadedMsg struct {
	DestPath  string
	Cancelled bool
	Err       error
}

// PresignedURLGeneratedMsg is emitted after a presigned GET URL is minted.
type PresignedURLGeneratedMsg struct {
	URL string
	Err error
}

// ObjectPreviewLoadedMsg is emitted after a ranged preview fetch completes.
type ObjectPreviewLoadedMsg struct {
	Content     []byte
	ContentType string
	Err         error
}
