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

func runMysqldump(ctx context.Context, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "mysqldump", "-u", "root", "-h", "127.0.0.1", "-P", "3306", "-pmy-secret-pw", "--all-databases")
	cmd.Stdout = writer
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return nil
}

func main() {
	initContainer()

	pipeReader1, pipeWriter1 := io.Pipe()
	pipeReader2, pipeWriter2 := io.Pipe()
	writer := io.MultiWriter(pipeWriter1, pipeWriter2)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runMysqldump(ctx, writer); err != nil {
			errCh <- err
		}
		pipeWriter1.Close()
		pipeWriter2.Close()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := processOutput("db_backup1.sql", pipeReader1); err != nil {
			errCh <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := processOutput("db_backup2.sql", pipeReader2); err != nil {
			errCh <- err
		}
	}()

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

func processOutput(filename string, r io.Reader) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create or open file %s: %w", filename, err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Printf("warning: failed to close file %s: %v", filename, cerr)
		}
	}()

	n, err := io.Copy(file, r)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %w", filename, err)
	}

	if n == 0 {
		log.Printf("warning: no data written to file %s", filename)
	}

	fmt.Printf("data count for %s: %d bytes\n", filename, n)
	return nil
}
