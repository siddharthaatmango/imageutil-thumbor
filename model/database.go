package model

type Analytic struct {
	ID           string
	UserID       string
	ProjectID    string
	ImageID      string
	UniqRequest  int64
	TotalRequest int64
	TotalBytes   int64
}

type Project struct {
	ID       string
	UserID   string
	Uuid     string
	Fqdn     string
	Protocol string
	BasePath string
}

type Image struct {
	ID             string
	UserID         string
	ProjectID      string
	Key            string
	Origin         string
	OriginPath     string
	Transformation string
	IsSmart        string
	CdnPath        string
	FileSize       int64
	ImgURL         string
}

type Folder struct {
	ID           string
	UserID       string
	ProjectID    string
	FolderID     string
	IsFile       string
	Name         string
	Path         string
	UploadToken  string
	OriginalName string
	MimeType     string
	FileSize     int64
}
