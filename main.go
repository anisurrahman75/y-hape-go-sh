package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func containerExists(name string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name="+name, "--format", "{{.Names}}")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to check if container exists: %v", err)
	}

	return strings.TrimSpace(out.String()) == name
}

func initContainer() {
	containerName := "mysql-container"

	if containerExists(containerName) {
		log.Printf("Container %s already exists. Skipping run command.", containerName)
		return
	}

	cmd := exec.Command("docker", "run", "--name", containerName,
		"-e", "MYSQL_ROOT_PASSWORD=my-secret-pw", "-p", "3306:3306", "-d", "mysql:latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run container: %v", err)
	} else {
		log.Printf("Container %s started successfully.", containerName)
	}
}

func runMysqldump(ctx context.Context, writer io.Writer, wg *sync.WaitGroup,
	pipeWriter1 *io.PipeWriter, pipeWriter2 *io.PipeWriter, errCh chan error) {
	defer wg.Done()
	defer func() {
		if cerr := pipeWriter1.Close(); cerr != nil {
			errCh <- fmt.Errorf("warning: failed to close pipeWriter1 %v", cerr)
		}
	}()

	defer func() {
		if cerr := pipeWriter2.Close(); cerr != nil {
			errCh <- fmt.Errorf("warning: failed to close pipeWriter2 %v", cerr)
		}
	}()

	cmd := exec.CommandContext(ctx, "mysqldump", "-u", "root", "-h", "127.0.0.1", "-P", "3306", "-pmy-secret-pw", "--all-databases")
	cmd.Stdout = writer
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		errCh <- fmt.Errorf("mysqldump failed: %w", err)
	}
}

func initResticRepos() {
	for _, repo := range resticRepos {
		cmd := exec.Command("restic", "snapshots", "-r", repo)
		cmd.Env = append(cmd.Env, "RESTIC_PASSWORD=my-secret-pw")

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			initCmd := exec.Command("restic", "init", "-r", repo)
			initCmd.Env = append(cmd.Env, "RESTIC_PASSWORD=my-secret-pw")
			initCmd.Stdout = os.Stdout
			initCmd.Stderr = os.Stderr
			if err := initCmd.Run(); err != nil {
				log.Printf("Failed to init restic repo %s: %v", repo, err)
			}
		}
	}
}

var resticRepos = []string{"repo-1", "repo-2"}

func main() {
	initContainer()
	initResticRepos()

	pipeReader1, pipeWriter1 := io.Pipe()
	pipeReader2, pipeWriter2 := io.Pipe()
	writer := io.MultiWriter(pipeWriter1, pipeWriter2)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error)

	wg.Add(1)
	go runMysqldump(ctx, writer, &wg, pipeWriter1, pipeWriter2, errCh)

	wg.Add(1)
	go processOutput(resticRepos[0], pipeReader1, &wg, errCh)

	wg.Add(1)
	go processOutput(resticRepos[1], pipeReader2, &wg, errCh)

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	fmt.Println("--end--")
}

func processOutput(repoName string, r io.Reader, wg *sync.WaitGroup, errCh chan error) {
	defer wg.Done()
	cmd := exec.Command("restic", "backup", "-r", repoName, "--stdin")
	cmd.Env = append(cmd.Env, "RESTIC_PASSWORD=my-secret-pw")
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		errCh <- fmt.Errorf("restic failed: %w", err)
	}
}
