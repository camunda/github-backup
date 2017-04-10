package main

import (
	"testing"
	"context"
	"fmt"
	"sync"
	"os"
)

func TestGithubBackupConstructor(t *testing.T) {
	backup := &GithubBackup{readConfig(), context.Background(), nil, sync.WaitGroup{}}
	if backup == nil {
		t.Fatal("Allocation failed.")
	}

	backup.login()
	repos, err := backup.getRepositories("camunda-ci")
	checkErr(err)

	if len(repos) < 50 {
		t.Fatal("Number of repositories is wrongs")
	}
}


func TestConfig(t *testing.T) {
	config := readConfig()
	if config == nil {
		t.Fatal("Reading configuration failed.")
	}
}

func TestCloneRepository(t *testing.T) {
	backup := &GithubBackup{readConfig(), context.Background(), nil, sync.WaitGroup{}}
	backup.login()

	repos, err := backup.getRepositories("camunda-ci")
	checkErr(err)
	backup.wg.Add(2)

	path := fmt.Sprintf(TMP_REPO_PATH, "camunda-ci", *(repos[0].Name))
	backup.cloneRepository(*(repos[0].CloneURL), path)

	path2 := fmt.Sprintf(TMP_REPO_PATH, "camunda-ci", *(repos[1].Name))
	backup.cloneRepository(*(repos[1].CloneURL), path2)

	os.RemoveAll(rootTmpDir)
}