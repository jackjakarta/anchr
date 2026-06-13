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

// DownloadPathChosenMsg is emitted after the native save dialog closes.
type DownloadPathChosenMsg struct {
	ClientIdx int
	Key       string
	DestPath  string
	Cancelled bool
	Err       error
}

// FileDownloadedMsg is emitted after a download completes (or fails).
type FileDownloadedMsg struct {
	DestPath string
	Err      error
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
