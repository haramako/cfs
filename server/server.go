package server

import (
	"crypto/md5"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Server struct {
	FpRoot    string `json:"root_filepath"`
	Port      int
	AdminUser string
	AdminPass string
}

func isHash(str string) bool {
	if len(str) != 32 {
		return false
	}
	for _, c := range str {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
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

func (s *Server) dataFilepath() string {
	return filepath.Join(s.FpRoot, "data")
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
	}
	body_hash := fmt.Sprintf("%x", md5.Sum(body))

	if hash != body_hash {
		c.AbortWithError(500, fmt.Errorf("invalid data hash %s", body_hash))
	}

	err = ioutil.WriteFile(filepath, body, 0666)
	if err != nil {
		c.AbortWithError(500, err)
	}

	c.String(200, "created")
}

func (s *Server) nonexists(c *gin.Context) {
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithError(500, err)
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

func (s *Server) Start() {
	r := gin.Default()
	r.StaticFS("/data", http.Dir(filepath.Join(s.FpRoot, "data")))
	r.StaticFS("/assets", http.Dir("./assets"))

	var admin *gin.RouterGroup
	if s.AdminUser == "" {
		admin = r.Group("/_admin")
	} else {
		accounts := gin.Accounts{s.AdminUser: s.AdminPass}
		admin = r.Group("/_admin", gin.BasicAuth(accounts))
	}
	fmt.Println(s)

	admin.StaticFile("/", "./assets/index.html")
	admin.POST("/upload/:hash", s.upload)
	admin.POST("/nonexists", s.nonexists)

	r.Run(fmt.Sprintf(":%d", s.Port))
}

func (s *Server) Init() error {

	err := os.MkdirAll(s.dataFilepath(), 0777)
	if err != nil {
		return fmt.Errorf("cannot create directory %s, cause %s", s.FpRoot, err)
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
