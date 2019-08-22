package cfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"

	//"net/url"
	"net/url"
	"path"
)

type CfsStorage struct {
	CabinetUrl *url.URL
}

func NewCfsStorage(cabinetUrl *url.URL) (*CfsStorage, error) {
	s := &CfsStorage{
		CabinetUrl: cabinetUrl,
	}
	return s, nil
}

func (s *CfsStorage) DownloaderUrl() *url.URL {
	return s.CabinetUrl
}

func (s *CfsStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {

	nonexistsRes, err := s.post("api/nonexists", []byte(hash))
	if err != nil {
		return err
	}

	if len(nonexistsRes) == 0 {
		return nil
	}

	_, err = s.post(path.Join("api/upload", hash), body)
	if err != nil {
		return err
	}

	//b.UploadCount++

	if Verbose {
		fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	}

	return nil
}

func (s *CfsStorage) post(location string, body []byte) ([]byte, error) {
	cli := &http.Client{}

	url, err := s.CabinetUrl.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("cannot parse url %s", location)
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cannot create request %s", url.String())
	}

	req.SetBasicAuth(Option.AdminUser, Option.AdminPass)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error request to %s, %s", url.String(), err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response code not 200 OK but %d, %s", resp.StatusCode, url.String())
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read body %s", url.String())
	}

	return respBody, nil
}

func (s *CfsStorage) UploadTag(filename string, body []byte) error {
	panic("not implemented")
}
