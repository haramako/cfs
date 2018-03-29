package cfs

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Server struct {
	RootFilepath string `json:"root_filepath"`
	Port         int
	AdminUser    string
	AdminPass    string
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
	return filepath.Join(s.RootFilepath, "data")
}

func (s *Server) tagsFilepath() string {
	return filepath.Join(s.RootFilepath, "tags")
}

func (s *Server) versionsFilepath() string {
	return filepath.Join(s.RootFilepath, "versions")
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
	bodyHash := fmt.Sprintf("%x", md5.Sum(body))

	if hash != bodyHash {
		c.AbortWithError(500, fmt.Errorf("invalid data hash %s", bodyHash))
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

	tagFiles, err := ioutil.ReadDir(s.tagsFilepath())
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	var tags []*TagFile
	for _, tagFile := range tagFiles {
		tagBytes, err := ioutil.ReadFile(s.tagFilepath(tagFile.Name()))
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

	newTag, err := TagFileFromReader(c.Request.Body)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	oldTag, err := TagFileFromFile(s.tagFilepath(id))
	if err == nil {
		if newTag.Hash == oldTag.Hash {
			c.String(200, "not modified")
			return
		}
	}

	tagFile, err := json.Marshal(newTag)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = ioutil.WriteFile(s.tagFilepath(id), tagFile, 0666)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = os.MkdirAll(filepath.Dir(s.versionFilepath(id)), 0777)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	err = ioutil.WriteFile(s.versionFilepath(id), tagFile, 0666)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.String(200, "tag created")
}

func (s *Server) getTagsFile(c *gin.Context) {
	id := c.Param("id")
	contentPath := c.Param("path")[1:] // '/' を消す
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

	content, found := bucket.Contents[contentPath]
	if !found {
		c.AbortWithError(500, fmt.Errorf("content %s not found", contentPath))
		return
	}

	contentBody, err := ioutil.ReadFile(s.hashFilepath(content.Hash))
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	contentBody, err = decode(contentBody, tag.EncryptKey, tag.EncryptIv, content.Attr)
	if err != nil {
		c.AbortWithError(500, err)
		return
	}

	c.Data(200, "text/plain", contentBody)
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

	hashList := strings.Split(string(body), "\n")
	result := make([]string, len(hashList))

	for _, hash := range hashList {
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
	err := filepath.Walk(s.RootFilepath, func(_ string, info os.FileInfo, err error) error {
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

	r.StaticFS("/data", http.Dir(filepath.Join(s.RootFilepath, "data")))
	r.StaticFS("/assets", http.Dir("assets"))

	var api, ui gin.IRoutes
	if s.AdminUser != "" {
		accounts := gin.Accounts{s.AdminUser: s.AdminPass}
		api = r.Group("/api", gin.BasicAuth(accounts))
		ui = r.Group("/ui", gin.BasicAuth(accounts))
	} else {
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
		file, err := ioutil.ReadFile(filepath.Join("assets", "index.html"))
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
		dirPath := filepath.Join(s.dataFilepath(), fmt.Sprintf("%02x", i))
		err = os.MkdirAll(dirPath, 0777)
		if err != nil {
			return fmt.Errorf("cannot create hash directory %s", dirPath)
		}
	}

	return nil
}
