package main

import (
	"strings"
	"sync"
	"archive/tar"
	"io"
	"time"
	"path"
	"context"
	"os"
	"fmt"
	"path/filepath"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"github.com/google/go-github/github"
	"gopkg.in/src-d/go-git.v4"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/client"
	gitHttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	gitTransport "gopkg.in/src-d/go-git.v4/plumbing/transport"
	"github.com/joho/godotenv"
)

const (
	TMP_REPO_PATH = "repositories/%s/%s"
)

var (
	rootTmpDir string = strings.Split(TMP_REPO_PATH, "/")[0]
	preserveDirStructureBool bool = true
)

type Config struct {
	S3Bucket string
	AwsAccessKey string
	AwsSecretAccessKey string
	AwsRegion string
	Username string
	Password string
	Organisations []string
}

func readConfig() *Config {
	err := godotenv.Load()
	checkErr(err)

	filename, _ := filepath.Abs("./config.yml")
	yamlFile, err := ioutil.ReadFile(filename)
	checkErr(err)

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)

	config.S3Bucket = os.Getenv("S3_BUCKET")
	config.AwsAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	config.AwsSecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	config.AwsRegion = os.Getenv("AWS_REGION")
	config.Username = os.Getenv("GITHUB_USERNAME")
	config.Password = os.Getenv("GITHUB_PASSWORD")

	return &config
}

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

func RenderTime(t time.Time) string {
	return t.Format("20060102-15:04:05.000")
}

func isDirectory(path string) bool {
	fd, err := os.Stat(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	switch mode := fd.Mode(); {
	case mode.IsDir():
		return true
	case mode.IsRegular():
		return false
	}
	return false
}

type GithubBackup struct {
	config *Config
	context context.Context
	client *github.Client
	wg         sync.WaitGroup
}

func (app *GithubBackup) uploadDirToS3(sess *session.Session, bucketName string, bucketPrefix string, dirPath string) {
	fileList := []string{}
	filepath.Walk(dirPath, func(path string, f os.FileInfo, err error) error {
		if isDirectory(path) {
			// Do nothing
			return nil
		} else {
			fileList = append(fileList, path)
			return nil
		}
	})

	app.wg.Add(len(fileList))
	for _, file := range fileList {
		go app.uploadFileToS3(sess, bucketName, bucketPrefix, file)
	}
	app.wg.Wait()
}

func (app *GithubBackup) uploadFileToS3(sess *session.Session, bucketName string, bucketPrefix string, filePath string) {
	defer app.wg.Done()
	fmt.Printf("[+] Spawning Upload routine: %s to S3.\n", filePath)

	// An S3 service
	s3Svc := s3.New(sess)
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Failed to open file", file, err)
		os.Exit(1)
	}
	defer file.Close()
	var key string
	if preserveDirStructureBool {
		key = bucketPrefix + filePath
	} else {
		key = bucketPrefix + path.Base(filePath)
	}
	// Upload the file to the s3 given bucket
	params := &s3.PutObjectInput{
		Bucket: aws.String(bucketName), // Required
		Key:    aws.String(key),        // Required
		Body:   file,
	}
	_, err = s3Svc.PutObject(params)
	if err != nil {
		fmt.Printf("Failed to upload data to %s/%s, %s\n",
			bucketName, key, err.Error())
		return
	}
}

func (app *GithubBackup) login() {
	auth := github.BasicAuthTransport{
		Username:app.config.Username,
		Password: app.config.Password,
		OTP: "", Transport: nil,
	}
	app.client = github.NewClient(auth.Client())
	client.InstallProtocol("https", gitHttp.NewClient(auth.Client()))
}

func (app *GithubBackup) getRepositories(organisation string) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []*github.Repository
	for {
		repos, resp, err := app.client.Repositories.ListByOrg(app.context, organisation, opt)
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}
	return allRepos, nil
}

func (app *GithubBackup) cloneRepository(cloneURL, path string) {
	defer app.wg.Done()
	fmt.Printf("[+] Spawning Clone routine: %s \n", cloneURL)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, os.ModePerm)
	}

	_, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:      cloneURL,
		Progress: os.Stdout,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
	})

	if err != nil && err != gitTransport.ErrEmptyRemoteRepository {
		checkErr(err)
	}

	app.compress(path, path+"/../")
	os.RemoveAll(path)
}

func (app *GithubBackup) downloadAll(organisation string) {
	repos, err := app.getRepositories(organisation)
	checkErr(err)

	app.wg.Add(len(repos))
	for _, repo := range repos {
		path := fmt.Sprintf(TMP_REPO_PATH, organisation, *repo.Name)
		go app.cloneRepository(*repo.CloneURL, path)
	}
	app.wg.Wait()
}

func (app *GithubBackup) compress(source, target string) error {
	filename := filepath.Base(source)
	target = filepath.Join(target, fmt.Sprintf("%s.tar", filename))
	tarFile, err := os.Create(target)
	checkErr(err)
	defer tarFile.Close()

	tarball := tar.NewWriter(tarFile)
	defer tarball.Close()

	info, err := os.Stat(source)
	checkErr(err)

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			checkErr(err)
			header, err := tar.FileInfoHeader(info, info.Name())
			checkErr(err)

			if baseDir != "" { header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source)) }
			err = tarball.WriteHeader(header)
			checkErr(err)

			if info.IsDir() { return nil }

			file, fsErr := os.Open(path)
			checkErr(fsErr)

			defer file.Close()
			_, err = io.Copy(tarball, file)
			checkErr(err)
			return nil
		})
}



func (app *GithubBackup) start() {
	backupTime := RenderTime(time.Now())

	fmt.Println("############################################################################")
	fmt.Printf("Starting a backup at %s.\n", backupTime)
	fmt.Printf("Using config %+v\n", app.config)
	fmt.Println("############################################################################")

	app.login()
	for _, org := range app.config.Organisations {
		app.downloadAll(org)
	}
	os.Rename("repositories", backupTime)
	app.uploadDirToS3(session.Must(session.NewSession()), app.config.S3Bucket, "", backupTime)

	//os.RemoveAll(rootTmpDir)
	os.RemoveAll(backupTime)
}

func main() {
	(&GithubBackup{readConfig(), context.Background(), nil, sync.WaitGroup{}}).start()
}
