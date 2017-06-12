package main

import (
	"strings"
	"sync"
	"archive/tar"
	"io"
	"time"
	"context"
	"os"
	"fmt"
	"path/filepath"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"github.com/google/go-github/github"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/joho/godotenv"
	"os/exec"
)

// constants definitions used by the app.
const (
	TMP_REPO_PATH = "%s/%s/%s"
	DATETIME_LAYOUT = "02-01-2006-15:04:05"
)

// Config is runtime configuration data used in GithubBackup.
type Config struct {
	S3Bucket string
	AwsAccessKey string
	AwsSecretAccessKey string
	AwsRegion string
	Username string
	Password string
	Organisations []string `yaml:"organisations"`
	KeepLastBackupDays int `yaml:"keep_last_backup_days"`
}

// printAll is small helper method which will print parts of configuration to stdout.
func (c *Config) printAll() {
	fmt.Println("S3Bucket: ", c.S3Bucket)
	fmt.Println("AwsAccessKey: ", c.AwsAccessKey)
	fmt.Println("AwsRegion: ", c.AwsRegion)
	fmt.Println("Github User: ", c.Username)
	fmt.Println("Organisations: ", c.Organisations)
}

func (c *Config) checkOrFail() {
	dirty := len(c.AwsAccessKey) == 0 || len(c.AwsSecretAccessKey) == 0 || len(c.AwsRegion) == 0
	dirty = dirty || len(c.S3Bucket) == 0 || len(c.Username) == 0 || len(c.Password) == 0

	if dirty {
		c.printAll()
		panic("[!] I'm missing configuration.")
	}
}

// readConfig will read .env file and config.yml to generate Config object for the runtime.
func readConfig() *Config {
	godotenv.Load()

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

	config.checkOrFail()
	return &config
}

// checkErr will always panic on error.
func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

// RenderTime is helper which will convert time.Time object to string.
func RenderTime(t time.Time) string {
	return t.Format(DATETIME_LAYOUT)
}

// Parse time is helper which will convert string into a time.Time objects. Both helpers use DATETIME_LAYOUT constant.
func ParseTime(t string) (time.Time, error) {
	return time.Parse(DATETIME_LAYOUT, t)
}

// GithubBackup contains all necessary elements to execute backup process.
type GithubBackup struct {
	config *Config
	context context.Context
	client *github.Client
	wg         sync.WaitGroup
	s3svc  *s3.S3
	createdAt string
}

// uploadFileToS3 will upload specified file to S3 bucket.
func (app *GithubBackup) uploadFileToS3(filePath string) {
	defer app.wg.Done()
	fmt.Printf("[+] Spawning S3UPLOAD routine: %s\n", filePath)

	file, err := os.Open(filePath)
	defer file.Close()
	checkErr(err)

	stat, _ := file.Stat()
	if stat.Size() == 0 { return } // file is empty. skip upload.

	params := &s3.PutObjectInput{ Bucket: aws.String(app.config.S3Bucket), Key: aws.String(filePath), Body: file, }
	_, err = app.s3svc.PutObject(params)

	if err != nil {
		fmt.Printf("Failed to upload data to %s/%s, %s\n", app.config.S3Bucket, filePath, err.Error())
		panic(err) // TODO: delete that backup try completely
	}
}

func (app *GithubBackup) getS3Page(marker string) (*s3.ListObjectsOutput, error)  {
	params := &s3.ListObjectsInput{
		Bucket: aws.String(app.config.S3Bucket),
	}

	if len(marker) > 0 {
		params.Marker = aws.String(marker)
	}

	return app.s3svc.ListObjects(params)
}

// cleanup method will delete old backups. Backup which are older then specified in config will be deleted.
func (app *GithubBackup) cleanup() {
	fmt.Println("[+] Starting CLEANUP.")

	os.RemoveAll(strings.Split(TMP_REPO_PATH, "/")[0])
	os.RemoveAll(app.createdAt)

	var objRefs []*s3.Object
	var marker string

	for {
		resp, _ := app.getS3Page(marker)
		lastKey := resp.Contents[len(resp.Contents) - 1].Key
		objRefs = append(objRefs, resp.Contents...)

		if *resp.IsTruncated == true {
			marker = *lastKey
		} else {
			break
		}
	}

	fmt.Printf("[+] Found %d objects for cleanup.\n", len(objRefs))
	for _, key := range objRefs {
		fmt.Println(*key.Key)
		ts, _ := ParseTime(strings.Split(*key.Key, "/")[0])
		if int(time.Since(ts).Hours())  > app.config.KeepLastBackupDays * 24 {
			fmt.Printf("[+] Found an old backup. Deleting %s\n", *key.Key)
			deleteParams := &s3.DeleteObjectInput{
				Bucket: aws.String(app.config.S3Bucket), Key: key.Key,
			}
			output, err := app.s3svc.DeleteObject(deleteParams)
			checkErr(err)
			fmt.Printf("\n\n[+/!] S3 Delete Object executed: %s\n\n", output)
		}
	}
}

// login method will use provided Github credentials and retrieve authentication token.
func (app *GithubBackup) login() {
	auth := github.BasicAuthTransport{
		Username:app.config.Username, Password: app.config.Password, OTP: "", Transport: nil,
	}
	app.client = github.NewClient(auth.Client())
}

// getRepositories will fetch all repositories for specified organisations in config.
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

// cloneRepository will download git repository from Github and compress them into a tarball.
func (app *GithubBackup) cloneRepository(repo *github.Repository, repoPath string) {
	fmt.Printf("[+] Trying to clone %s.\n", *repo.FullName)

	cloneUrl := *repo.CloneURL
	credentialsUrl := fmt.Sprintf("https://%s:%s@%s",
						os.Getenv("GITHUB_USERNAME"), os.Getenv("GITHUB_PASSWORD"),
						cloneUrl[8:])

	cmd := exec.Command("git", "clone", "--mirror", credentialsUrl, repoPath)
	if err := cmd.Run(); err != nil {
		fmt.Println("[!] git error: ", err)
		fmt.Println(">> git clone ", *repo.CloneURL, repoPath)
		return
	}

	gitRepoCleanup := fmt.Sprintf("cd %s && git remote rm origin", repoPath)
	rmRemote := exec.Command("/bin/sh", "-c", gitRepoCleanup) // Don't backup credentials.
	if err := rmRemote.Run(); err != nil {
		fmt.Printf("[!] cannot remove remote: %+#v\n", err)
	}

	app.compress(repoPath, repoPath+"/../")
	os.RemoveAll(repoPath)
	repoBundle := fmt.Sprintf("%s.tar", repoPath)
	app.uploadFileToS3(repoBundle)

}

// downloadAll will fetch all repository endpoints for a given organisation and clone them to filesystem.
func (app *GithubBackup) downloadAll(organisation string) {
	repos, err := app.getRepositories(organisation)
	if err != nil { return }

	for _, repo := range repos {
		path := fmt.Sprintf(TMP_REPO_PATH, app.createdAt, organisation, *repo.Name)
		fmt.Printf("[+] Spawning GIT_CLONE routine: %s \n", *repo.CloneURL)
		app.wg.Add(1)
		go app.cloneRepository(repo, path)
	}
}

// compress will create tarballs of cloned repositories.
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

// start is a helper method which will execute the backup process.
func (app *GithubBackup) start() {
	fmt.Println("############################################################################")
	fmt.Printf("[+] Starting a backup at %s.\n", app.createdAt)
	app.config.printAll()
	fmt.Println("############################################################################")

	app.login()
	for _, org := range app.config.Organisations {
		app.downloadAll(org)
	}

	app.wg.Wait()
	app.cleanup()
}

// NewGithubBackup is a construct function which will create new GithubBackup object with given attributes.
func NewGithubBackup() *GithubBackup {
	return &GithubBackup{
		readConfig(),
		context.Background(),
		nil,
		sync.WaitGroup{},
		s3.New(session.Must(session.NewSession())),
		RenderTime(time.Now()),
	}
}

func main() {
	NewGithubBackup().start()
}
