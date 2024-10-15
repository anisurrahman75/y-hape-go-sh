package main

import (
	"fmt"
	"io"
	"log"
	"strings"
)

func main() {
	pipeReader1, pipeWriter1 := io.Pipe()
	pipeReader2, pipeWriter2 := io.Pipe()

	writer := io.MultiWriter(pipeWriter1, pipeWriter2)

	// Start a goroutine to write data into both pipes.
	go func() {
		n, err := io.Copy(writer, strings.NewReader("Anisur Rahman"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("----N:", n)

		// Close both writers to unblock readers.
		pipeWriter1.Close()
		pipeWriter2.Close()
	}()

	// Run both tr1 and tr2 concurrently.
	done := make(chan bool) // Channel to wait for both tr1 and tr2 to finish.

	go func() {
		tr1(pipeReader1, "e", "i")
		done <- true
	}()

	go func() {
		tr2(pipeReader2, "e", "i")
		done <- true
	}()

	// Wait for both readers to finish.
	<-done
	<-done

	fmt.Println("--end--")
}

func tr1(r io.Reader, old string, new string) {
	data, _ := io.ReadAll(r)
	res := strings.Replace(string(data), old, new, -1)
	fmt.Println("tr1 result:", res)
}

func tr2(r io.Reader, old string, new string) {
	data, _ := io.ReadAll(r)
	res := strings.Replace(string(data), old, new, -1)
	fmt.Println("tr2 result:", res)
}
