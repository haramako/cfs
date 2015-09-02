package cfs

type Uploader interface {
	Upload(path string, body []byte, overwrite bool) error
	Close()
}

type EmptyUploader struct {
}

func CreateEmptyUploader(info string) (*EmptyUploader, error) {
	return new(EmptyUploader), nil
}

func (u *EmptyUploader) Upload(path string, body []byte, overwrite bool) error {
	return nil
}

func (u *EmptyUploader) Close() {
}
