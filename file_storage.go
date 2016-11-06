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

type FileStorage struct {
	CabinetUrl *url.URL
}

func (s *FileStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {

	nonexists_res, err := s.post("api/nonexists", []byte(hash))
	if err != nil {
		return err
	}

	if len(nonexists_res) == 0 {
		return nil
	}

	_, err = s.post(path.Join("api/upload", hash), body)
	if err != nil {
		return err
	}

	//b.UploadCount++

	//if Verbose {
	fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	//}

	return nil
}

func (s *FileStorage) post(location string, body []byte) ([]byte, error) {
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

	resp_body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read body %s", url.String())
	}

	return resp_body, nil
}
