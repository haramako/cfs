package cfs

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	FpRoot    string `json:"root_filepath"`
	Port      int
	AdminUser string
	AdminPass string
}

func (s *Server) hashPath(hash string) string {
	if !isHash(hash) {
		panic("invalid hash " + hash)
	}
	return hash[0:2] + "/" + hash[2:]
}

func (s *Server) hashFilepath(hash string) string {
	return filepath.Join(s.dataFilepath(), filepath.FromSlash(s.hashPath(hash)))
}

func (s *Server) tagFilepath(tag string) string {
	return filepath.Join(s.tagsFilepath(), tag)
}

func (s *Server) versionFilepath(tag string) string {
	now := time.Now().Format("2006-01-02-150405")
	return filepath.Join(s.versionsFilepath(), tag, now)
}

func (s *Server) versionListFilepath(tag string) string {
	return filepath.Join(s.versionsFilepath(), tag)
}

func (s *Server) dataFilepath() string {
	return filepath.Join(s.FpRoot, "data")
}

func (s *Server) tagsFilepath() string {
	return filepath.Join(s.FpRoot, "tags")
}

func (s *Server) versionsFilepath() string {
	return filepath.Join(s.FpRoot, "versions")
}

func (s *Server) upload(c *gin.Context) {
	hash := c.Param("hash")
	filepath := s.hashFilepath(hash)

	stat, err := os.Stat(filepath)
	if err != nil && stat != nil {
		c.String(200, "already exists")
		return
	}

	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}
	body_hash := fmt.Sprintf("%x", md5.Sum(body))

	if hash != body_hash {
		c.AbortWithError(500, fmt.Errorf("invalid data hash %s", body_hash))
		return
	}

	err = ioutil.WriteFile(filepath, body, 0666)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.String(200, "created")
}

func (s *Server) indexTags(c *gin.Context) {

	tag_files, err := ioutil.ReadDir(s.tagsFilepath())
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	var tags []*TagFile
	for _, tag_file := range tag_files {
		tagBytes, err := ioutil.ReadFile(s.tagFilepath(tag_file.Name()))
		if err != nil {
			c.AbortWithError(500, err)
			return
		}

		var tag TagFile
		err = json.Unmarshal(tagBytes, &tag)
		if err != nil {
			c.AbortWithError(500, err)
			return
		}

		tags = append(tags, &tag)
	}

	c.JSON(200, tags)
}

func (s *Server) getTags(c *gin.Context) {
	id := c.Param("id")

	tag, err := TagFileFromFile(s.tagFilepath(id))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	bucket, err := tag.Bucket(s)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	files := []string{}
	for k := range bucket.Contents {
		files = append(files, k)
	}

	c.JSON(200, map[string]interface{}{"tag": tag, "files": files})
}

func (s *Server) postTags(c *gin.Context) {
	id := c.Param("id")

	new_tag, err := TagFileFromReader(c.Request.Body)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	old_tag, err := TagFileFromFile(s.tagFilepath(id))
	if err == nil {
		if new_tag.Hash == old_tag.Hash {
			c.String(200, "not modified")
			return
		}
	}

	tag_file, err := json.Marshal(new_tag)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = ioutil.WriteFile(s.tagFilepath(id), tag_file, 0666)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = os.MkdirAll(filepath.Dir(s.versionFilepath(id)), 0777)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = ioutil.WriteFile(s.versionFilepath(id), tag_file, 0666)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.String(200, "tag created")
}

func (s *Server) getTagsFile(c *gin.Context) {
	id := c.Param("id")
	content_path := c.Param("path")[1:] // '/' を消す
	tag, err := TagFileFromFile(s.tagFilepath(id))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	bucket, err := tag.Bucket(s)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	content, found := bucket.Contents[content_path]
	if !found {
		c.AbortWithError(500, fmt.Errorf("content %s not found", content_path))
		return
	}

	content_body, err := ioutil.ReadFile(s.hashFilepath(content.Hash))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	content_body, err = decode(content_body, tag.EncryptKey, tag.EncryptIv, content.Attr)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "text/plain", content_body)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *Server) indexTagsVersions(c *gin.Context) {
	id := c.Param("id")

	files, err := ioutil.ReadDir(s.versionListFilepath(id))
	check(err)

	versions := []string{}
	for _, file := range files {
		versions = append(versions, file.Name())
	}

	c.JSON(200, versions)
}

func (s *Server) nonexists(c *gin.Context) {
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	hash_list := strings.Split(string(body), "\n")
	result := make([]string, len(hash_list))

	for _, hash := range hash_list {
		_, err := os.Stat(s.hashFilepath(hash))
		if err != nil && os.IsNotExist(err) {
			result = append(result, hash)
		}
	}
	c.String(200, strings.Join(result, "\n"))
}

func (s *Server) stat(c *gin.Context) {
	var r struct {
		TotalSize int64 `json:"totalSize"`
		FileCount int   `json:"fileCount"`
	}
	err := filepath.Walk(s.FpRoot, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		r.TotalSize += info.Size()
		r.FileCount++
		return nil
	})
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.JSON(200, r)
}

func (s *Server) Start() {
	r := gin.Default()

	r.StaticFS("/data", http.Dir(filepath.Join(s.FpRoot, "data")))
	r.StaticFS("/assets", http.Dir("./assets"))

	var api, ui gin.IRoutes
	if s.AdminUser != "" {
		accounts := gin.Accounts{s.AdminUser: s.AdminPass}
		api = r.Group("/api", gin.BasicAuth(accounts))
		ui = r.Group("/ui", gin.BasicAuth(accounts))
	}else{
		api = r.Group("/api")
		ui = r.Group("/ui")
	}
	
	{
		api.POST("/upload/:hash", s.upload)
		api.POST("/nonexists", s.nonexists)
		api.GET("/stat", s.stat)
		api.GET("/tags", s.indexTags)
		api.GET("/tags/:id", s.getTags)
		api.POST("/tags/:id", s.postTags)
		api.GET("/tags/:id/files/*path", s.getTagsFile)
		api.GET("/tags/:id/versions", s.indexTagsVersions)
	}

	ui.GET("/*dummy", func(c *gin.Context) {
		file, err := ioutil.ReadFile("./assets/index.html")
		if err != nil {
			c.AbortWithError(500, err)
			return
		}
		c.Data(200, "text/html", file)
	})

	// redirect _admin/* to api/*
	r.Any("/_admin/*path", func(c *gin.Context) {
		c.Request.URL.Path = "/api" + c.Param("path")
		r.ServeHTTP(c.Writer, c.Request)
	})

	r.Run(fmt.Sprintf(":%d", s.Port))
}

func (s *Server) Init() error {

	err := os.MkdirAll(s.dataFilepath(), 0777)
	if err != nil {
		return err
	}

	err = os.MkdirAll(s.tagsFilepath(), 0777)
	if err != nil {
		return err
	}

	err = os.MkdirAll(s.versionsFilepath(), 0777)
	if err != nil {
		return err
	}

	for i := 0; i < 256; i++ {
		dir_path := filepath.Join(s.dataFilepath(), fmt.Sprintf("%02x", i))
		err = os.MkdirAll(dir_path, 0777)
		if err != nil {
			return fmt.Errorf("cannot create hash directory %s", dir_path)
		}
	}

	return nil
}
