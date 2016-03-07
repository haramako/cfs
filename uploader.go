package cfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type Uploader struct {
	Url         *url.URL
	UploadCount int
}

func CreateUploader(rawurl string) (*Uploader, error) {
	_url, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	return &Uploader{Url: _url}, nil
}

func (u *Uploader) Upload(_path string, body []byte, overwrite bool) error {
	_path = strings.Replace(_path, "data", "", -1)
	_path = strings.Replace(_path, "/", "", -1)
	upload_url, _ := u.Url.Parse(path.Join("upload", _path))

	nonexists_url, _ := u.Url.Parse("nonexists")
	resp0, err := http.Post(nonexists_url.String(), "application/octetstream", strings.NewReader(_path))
	if err != nil {
		return fmt.Errorf("cannot upload file0 %s", _path)
	}
	defer resp0.Body.Close()

	body0, err := ioutil.ReadAll(resp0.Body)
	if err != nil {
		return fmt.Errorf("cannot read body0 %s", nonexists_url)
	}
	if len(body0) == 0 {
		return nil
	}

	resp, err := http.Post(upload_url.String(), "application/octetstream", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot upload file %s", _path)
	}
	defer resp.Body.Close()

	u.UploadCount++

	if Verbose {
		fmt.Printf("uploading '%s'\n", _path)
	}

	return nil
}
